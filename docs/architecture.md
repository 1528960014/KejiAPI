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

### Web 控制台（M3）
- 多模型对比聊天（同一 prompt 并排 N 个模型，SSE 流式渲染）
- 生成工作台（图/视频/音乐/TTS 表单 + 任务列表 + 结果预览）
- API 控制台（key 管理、用量与消费）
- i18n zh-CN/en-US；深浅主题

## 核心数据表

users、ledger_entries、models、channels、api_keys（含 P2-3 agent_id/markup）、tasks、dramas、usage_logs、agents

## 技术选型

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 网关语言 | Go (Gin) | 流式代理性能、单二进制部署 |
| 前端 | Vue3 + Vite + TS + Element Plus + vue-i18n | 生态与贡献者熟悉度 |
| 数据库 | PostgreSQL 16 | JSONB、事务、成熟 |
| 任务队列 | DB 轮询（`FOR UPDATE SKIP LOCKED`） | v1 量级够用，少一个依赖；Redis 留作限流等后续 |
| 媒体存储 | MinIO (S3 API) | 产物持久化（后续里程碑）；当前直接返回上游 URL |
| 许可证 | MIT | 采用率优先 |