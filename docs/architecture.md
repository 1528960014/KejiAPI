# 架构设计

## 总体结构

```
浏览器 (Vue3 SPA)
   │ HTTPS
   ▼
Nginx/Traefik（静态资源 + 反代）
   └── /v1/*、/admin/*  →  Gateway (Go, 单二进制)
        ├── 认证（API key / MASTER_KEY）
        ├── 模型网关
        │    ├── Provider 适配器（OpenAI 兼容透传；Anthropic 等后续）
        │    ├── 路由：model → channel（priority + 故障切换）
        │    └── 计费钩子（估算→冻结→结算，M2）
        ├── 任务管线（M4：DB 轮询 worker → provider 适配器 → 上游任务 → 轮询产物）
        ├── 漫剧管线（P2-1：drama worker → LLM 分镜 → 逐镜复用 M4 适配器 → 素材包）
        └── 管理 API（用户、key、模型、通道、站点配置）

PostgreSQL（用户/账本/模型/通道/任务队列）  MinIO（媒体产物，后续）
```

## 模块说明

### 模型网关（M1）
- **model**：对外模型 ID、provider、上游模型名、能力标签、计价（每 1K token）。
- **channel**：同一 model 的上游接入点（base_url + api_key + priority），可多通道故障切换。
- 请求流：API key 鉴权 → 查 model → 选 channel → 替换 body 中 model 为上游名 → 透传转发（SSE 直写回）→ 记录 usage。

### 计费（M2）
- 账本式：`users.balance`（微美元整数，1 USD = 10^6 µ）+ `ledger_entries`（credit/hold/release/debit 有符号流水），余额 = 流水汇总，同事务更新。
- 请求生命周期：预估费用（prompt ≈ 字节数/4，completion ≈ max_tokens，默认 1024 上限 16384）→ 行锁下检查余额并 hold → 完成后 release + 按实际 token debit（chat 按真实 usage；流式注入 `stream_options.include_usage` 取末 chunk usage，取不到则按预估计费）→ 失败全额 release。
- 实际超过冻结额时允许透支（余额可为负），账本始终一致。
- API key 绑定用户 + 模型白名单（allowed_models）+ 额度（quota 微美元，消费累计在 api_keys.spend）+ 过期时间；`sk-` 前缀，库内只存 SHA-256。
- 管理端：创建用户、手动充值（ledger credit）、账本流水查询、用量明细/汇总。

### 任务管线（M4）
- task 表（uuid、type、status、payload、hold_micro、result_urls、cost）；**DB 轮询队列**（`FOR UPDATE SKIP LOCKED` 认领，2s 间隔，多实例天然安全），不引入 Redis（有意偏离早期草案，v1 量级够用）。
- 适配器按 `channel.provider` + 任务类型注册：image 走 OpenAI 兼容 `/images/generations`；video 走 dashscope 原生异步（`X-DashScope-Async` + `/api/v1/tasks/{id}` 轮询）与 kling（`/v1/videos/text2video` + 同路径轮询）；tts 走 dashscope CosyVoice（同步）；music 暂无适配器（任务以明确错误失败）。
- 计费：提交时冻结 `unit_price × n`（models 表 `price_unit`/`unit_price`）；worker 成功结算、失败/超时（默认 10min）退回；进程重启时残留 running 任务标记失败并退回。
- 产物直接用上游返回的 URL；`b64_json` 结果明确报错（产物下载至 MinIO 并返回签名 URL 为后续里程碑）。
- 统一轮询 `/v1/media/status/{task_id}`（仅属主 key 可读）；管理端 `GET /admin/tasks`。

### 漫剧管线（P2-1）
- `dramas` 单表：script/style/模型选择/status/hold_micro + `shots JSONB`（分镜数组即素材包）。
- 独立的 drama worker（与 media worker 同套路：SKIP LOCKED 认领、启动恢复、超时 30min）。
- 分镜：配置了 `storyboard_model`（chat 模型）时走网关自己的 chat 通道，system prompt 约束严格 JSON（容忍 code fence）；未配置时按句子/段落简单拆分。
- 逐镜头复用 M4 的 image/tts 适配器；每完成一镜更新 JSONB（前端轮询可见进度）。
- 计费 all-or-nothing：提交时冻结 `shots × (图像+配音 单价)`，全部成功结算、任一失败全额退回；drama UUID 为账本 request_id。
- 产物：JSON 素材包（shots + URL，前端可下载）。
- **成片（P2-1b）**：结算后由 `Composer`（task 包）调主机上的 `ffmpeg`/`ffprobe`：下载各镜素材 → 每镜渲染一段固定 1280x720@30 的 h264+aac 片段（图像信箱铺满、台词 SRT 字幕烧录、无配音补静音、时长=配音+0.5s）→ concat demuxer 免重编码拼接 → `MEDIA_DIR/dramas/{uuid}.mp4`，经 `GET /media/dramas/:uuid` 公开服务（UUID 即凭证）。渲染失败只记 `video_error`，不动钱、不动素材包。

### 代理分销（P2-3）
- `agents` 表：user_id 唯一 + 批发系数 `rate ∈ (0,1]`。`GetUser` LEFT JOIN 出 `AgentRate`，chat/media/drama 三个计费入口在 hold 前统一 `billing.ApplyRate`（向上取整、正数不断零）；worker 按冻结额原样结算，无需改动。
- 子 key：`api_keys.agent_id`（FK → agents，`ON DELETE SET NULL`）+ `markup NUMERIC`（仅展示用的客户加价倍率）。子 key 的 `user_id` 指向代理用户，因此消费自然落入代理余额、按代理批发价结算；每个子 key 独立 allowed_models/quota/expires。
- 权限边界：代理控制台（`/api/me/subkeys`，JWT）只能建/删自己的子 key；代理自己的 `/api/me/keys` 列表与删除均排除子 key（`agent_id IS NULL`），互不越权。
- 业务模型：代理预购制——代理按零售价充值、按批发价消费，与客户之间的收款（markup）在平台之外进行；平台不向终端客户收款（在线支付为 P2-4）。
- 撤销代理（DELETE /admin/agents/:id）只解除代理身份，子 key 降级为代理用户的普通 key。

### 智能体（P2-2，预定义 agent 模板）
- `assistants` 表：公开 `agent_id` + 名称/描述/system prompt + 绑定的真实 `model_id` + `tools`（JSONB，OpenAI 函数定义数组，可空）+ enabled；首次启动播种 3 个内置模板（翻译官/写手/客服），绑定当时第一个启用的 chat 模型（幂等，可改绑）。
- 复用 chat 通道：`/v1/chat/completions` 的 `model` 命中 agent_id 时，`injectSystemPrompt` 在 messages 头部注入模板 system prompt，`injectTools` 注入模板 tools（请求自带 `tools` 时不覆盖），随后按真实模型取通道、估价（注入后的 body）、hold/settle——worker、账本、用量日志零改动；usage 日志记真实模型。
- key 白名单：agent id 或绑定模型任一命中即放行；空白名单全放行。
- 无状态多轮：历史由客户端维护（与现有 chat 页一致）；`GET /v1/agents` 供前端/SDK 列模板（含 `tools`，供客户端实现工具执行）。
- 工具调用（P3-1）：网关只负责把模板 `tools` 注入上游请求，`tool_calls` 随响应原样透传（含流式 `delta.tool_calls`）；**工具由客户端执行**（网关不执行工具、不做 MCP），客户端把 `role:"tool"` 结果加入 messages 续问。Web 聊天页把 tool_calls 渲染为可读文本、不执行。
- 管理端 `/admin/assistants` CRUD（改绑模型校验存在性，`tools` 校验为含 `function` 对象的数组）；Web 聊天页"智能体"下拉选择后锁定模型列。

### 在线支付（P2-4，人民币充值 → USD 余额）
- 订单模型：`recharges` 表，商户订单号 `order_no`（`RH…`，DB 唯一）+ 渠道（yipay/alipay/wechat）+ 金额（分）+ **下单时锁定**的入账 `credit_micro`（= 金额 / `PAY_CNY_PER_USD`），渠道后改汇率不影响在途订单。
- 渠道抽象 `internal/pay`：`Channel{ID, Name, CreateOrder, ParseNotify}` 三实现——
  - `YiPay`（开源易支付 V1 mapi）：MD5 签名（参数 ASCII 排序拼 `k=v&`，末尾直接拼商户密钥）；下单 `POST mapi.php` 得 `payurl`/`qrcode`；回调 GET/POST，验签后应答纯文本 `success`。
  - `Alipay`（官方当面付）：`alipay.trade.precreate` 取 `qr_code`；RSA2（SHA256withRSA）对原始参数值签名，回调验支付宝公钥 + 校验 app_id/TRADE_SUCCESS。需要商户开通"当面付"产品。
  - `Wechat`（官方 v3 Native）：`POST /v3/pay/transactions/native` 取 `code_url`；请求用商户 API 证书私钥签名（WECHATPAY2-SHA256-RSA2048），回调验微信**平台公钥**签名 + AES-256-GCM（APIv3Key）解密 resource。
- 入账事务：回调验签 → 订单存在且 method 匹配、金额（分）一致、`status='pending'` → 单事务翻转 paid + ledger `credit`（request_id = order_no，ledger 唯一索引兜底）+ 用户余额。重复回调返回成功但不重复入账。
- 配置（P3-2 起）：渠道凭据存 `pay_settings`（汇率/公网地址）+ `pay_channels`（每渠道 enabled + 凭据 JSONB）两表。**env `PAY_*` 只是首次启动的初始默认**（`api.ReloadPayConfig` 播种一次），之后管理端 `GET/PUT /admin/pay-config` 是唯一事实来源，保存后**热加载**（内存态整体换锁替换，读写互斥，无需重启）。凭据缺失/非法的渠道不启用，`status.error` 说明原因（如 RSA 解析失败），不影响其他渠道。`PAY_PUBLIC_URL`/`public_url` 指定回调可达的外网地址（缺省用请求 Host）。未做退款（充值类订单通常线下处理）。
- Web：`/recharge` 页面（金额预设+自定义、渠道选择、易支付内部支付宝/微信二选一、QR 码渲染、3s 轮询最长 5 分钟、充值记录列表）；管理端 `GET /admin/recharges` 全平台订单审计。

### Web 控制台（M3）
- 多模型对比聊天（同一 prompt 并排 N 个模型，SSE 流式渲染）
- 生成工作台（图/视频/音乐/TTS 表单 + 任务列表 + 结果预览）
- API 控制台（key 管理、用量与消费）
- 充值页（P2-4：人民币充值，渠道 QR/跳转、自动到账轮询）
- i18n 六语言（P2-5：zh-CN/en-US/ja/ko/ru/es，顶栏下拉切换、localStorage 记忆）；深浅主题

## 核心数据表

users、ledger_entries、models、channels、api_keys（含 P2-3 agent_id/markup）、tasks、dramas、usage_logs、agents、assistants、recharges

## 技术选型

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 网关语言 | Go (Gin) | 流式代理性能、单二进制部署 |
| 前端 | Vue3 + Vite + TS + Element Plus + vue-i18n | 生态与贡献者熟悉度 |
| 数据库 | PostgreSQL 16 | JSONB、事务、成熟 |
| 任务队列 | DB 轮询（`FOR UPDATE SKIP LOCKED`） | v1 量级够用，少一个依赖；Redis 留作限流等后续 |
| 媒体存储 | MinIO (S3 API) | 产物持久化（后续里程碑）；当前直接返回上游 URL |
| 许可证 | MIT | 采用率优先 |