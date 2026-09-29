# API 参考

## 鉴权

- 开放接口（`/v1/*`）：`Authorization: Bearer sk-xxxx`（API key）
- 管理接口（`/admin/*`）：`Authorization: Bearer $MASTER_KEY`
- 账号接口（`/api/me*`）：`Authorization: Bearer <access_token>`（JWT，由登录/注册接口签发）

## 计费模型（M2）

- 金额单位：账本内部以**微美元**（micro-USD，1 USD = 1,000,000 µ）整数存储，保证流水精确；管理接口用 USD 浮点（`*_usd` 字段）输入输出，同时提供 `*_micro` 整数字段。
- **余额计费**：API key 绑定用户（`user_id`）后按余额扣费。请求生命周期：预估费用 → 检查余额并冻结（hold）→ 上游返回后释放冻结、按实际 token 结算（settle）；请求失败则全额退回冻结。
- **402** `insufficient_balance`：余额不足以覆盖预估费用。
- **429** `quota_exceeded`：key 设置了 `quota_usd`，且历史消费 + 本次预估超过额度。未绑定用户的 key 不扣费，但 quota 仍然生效（消费累计在 `api_keys.spend`）。
- **预估启发式**：prompt ≈ messages 文本总字节数 / 4（向上取整，对中文偏保守高估）；completion ≈ 请求里的 `max_tokens`（缺省 1024，上限 16384）。
- **流式结算**：网关对 `stream: true` 的注入 `stream_options.include_usage=true`；上游最后一个 chunk 带回 usage 时按真实 token 结算，否则按预估计费（`usage_logs.status = ok_estimated`）。
- **透支**：实际费用超过冻结额时允许余额为负（账本始终一致）。
- **代理分销（P2-3）**：管理员可将某用户标记为代理并设批发系数 `rate ∈ (0, 1]`（如 0.85 = 八五折）。该用户**所有**冻结与结算金额都按 `ceil(金额 × rate)` 打折（正数永不断为 0）；代理可在控制台创建"子 key"分发给客户——子 key 共用代理余额、按代理批发价结算，各自独立的模型白名单 / 额度 / 有效期。`markup`（加价倍率）仅为代理向客户展示的建议价，平台不向终端客户收款。
- **智能体（P2-2）**：`/v1/chat/completions` 的 `model` 可以填**智能体模板 id**（如 `agent-translator`）。网关识别后：把模板的 system prompt 注入到 messages 最前 → 走模板绑定的**真实模型**的通道与计费（按真实模型的 token 单价）；key 白名单校验"agent id 或真实模型"任一命中即可。无状态：多轮上下文由客户端保留（与现有 chat 一致）。首次启动自动播种 3 个内置模板（翻译官 / 写手 / 客服），绑定当时第一个启用的 chat 模型，可用 `/admin/assistants` 改绑 / 增删。
- **智能体工具调用（P3-1）**：智能体模板可携带 OpenAI 格式的 `tools`（function 定义数组）。以 agent id 发起 chat 时，网关把模板的 `tools` 注入上游请求体（**请求自带 `tools` 时以请求为准**），上游原样流式回传 `tool_calls`；工具**由客户端执行**并携带结果续问（标准 function calling 流程），网关本身不执行工具、不做 MCP。SDK 客户端可直接拿到模板 tools 定义（`GET /v1/agents` 返回 `tools` 字段）。
- **在线充值（P2-4）**：用户通过易支付 / 支付宝官方 / 微信支付官方以人民币充值，按 `PAY_CNY_PER_USD` 汇率在**下单时**锁定入账的 micro-USD；渠道异步回调验签（易支付 MD5、支付宝 RSA2、微信 v3 平台密钥 + AES-256-GCM）后，在单事务内把订单置为 paid 并记 `credit` 流水入账。入账前校验渠道金额与订单金额一致；幂等，重复回调不重复入账。

## 开放接口

### GET /v1/models

返回启用的模型列表（OpenAI 兼容格式）。

### GET /v1/agents（P2-2 智能体）

需要 API key 鉴权。返回启用的智能体模板：`agent_id`、`name`、`description`、`model`（绑定的真实模型）、`tools`（模板的工具定义数组，OpenAI 格式；无工具时为 null）。

调用方式：`POST /v1/chat/completions`，`"model": "agent-translator"` 即可；system prompt 与模板的 `tools` 由网关注入，计费按绑定的真实模型。带工具的模板会触发上游 function calling，响应（含流式 `delta.tool_calls`）原样透传，客户端执行工具后把 `role:"tool"` 结果加入 messages 续问即可。

非标准附加字段（OpenAI SDK 会忽略）：`input_price_per_1k` / `output_price_per_1k`（token 计价），`price_unit`（`token` 默认 / `image` / `video` / `music` / `tts`）与 `unit_price`（每件 USD，媒体模型用）。

### POST /v1/chat/completions

OpenAI 兼容。`stream: true` 时返回 SSE。请求体其余字段原样透传上游（流式请求会被注入 `stream_options.include_usage`）。

```json
{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "hi"}],
  "stream": false
}
```

错误：

- 401 `invalid_api_key` / `expired_api_key`
- 403 `model_not_allowed`（key 的 `allowed_models` 不含该模型）、`user_disabled` / `user_not_found`
- 404 `model_not_found`
- 402 `insufficient_balance`
- 429 `quota_exceeded`
- 429 `rate_limited`（P4-3：网关级 RPM/TPM 限流触发，见"限流"）
- 502 `no_channel` / `upstream_error`

### POST /v1/media/generate（M4）

异步媒体生成（`image` / `video` / `music` / `tts`），API key 鉴权。提交成功返回 `202 {task_id, status: "queued"}`，用 `GET /v1/media/status/{task_id}` 轮询。

```json
{
  "model": "flux-1",
  "type": "image",          // 可省略：模型只有一个媒体能力时自动推断
  "prompt": "a cat astronaut",
  "n": 1,                   // 1-4，默认 1；video/music/tts 忽略
  "size": "1024x1024"       // image 可选
}
```

- video 额外支持 `duration`（字符串秒数，如 `"5"`）；tts 用 `text` 字段而不是 `prompt`。
- 计费：提交时冻结 `unit_price × n`（`unit_price` 为每件 USD），worker 成功后结算、失败/超时全额退回（失败不扣费）。
- 支持的 provider × 类型（适配器矩阵）：
  - `image`：所有 OpenAI 兼容 `/images/generations` 的 provider（openai、硅基流动、dashscope 兼容模式、自建网关等）
  - `video`：`dashscope`（原生异步 + 任务轮询）、`kling`（异步 + 任务轮询）
  - `tts`：`dashscope`（CosyVoice 同步）
  - `music`：`suno`（**社区 API，非 Suno 官方**，自备 key 与 base_url；异步提交 + 任务轮询，成功后 `audio_url` 进 `result_urls`）
- 错误：400 `invalid_request`（缺 model/type/prompt、模型无该能力、无媒体价格）；401/403/404/402/429 同 chat 接口；429 时冻结已退回。

### GET /v1/media/status/{task_id}

查询任务状态。仅持有该任务的 API key 可读（403 `forbidden`）。

```json
{
  "task_id": "…",
  "status": "queued | running | succeeded | failed",
  "type": "image",
  "model": "flux-1",
  "result_urls": ["https://…"],
  "cost_usd": 0.02,
  "error": "",
  "created_at": "…", "updated_at": "…"
}
```

- 上游返回 `b64_json`（而非 URL）时任务失败并提示（产物持久化到 MinIO 为后续里程碑）。
- 任务默认超时 10 分钟（超时按失败处理，冻结退回）；服务重启时残留的 running 任务会被标记失败并退回。

### POST /v1/drama/generate（P2-1 漫剧）

异步漫剧生成：剧本 → LLM 分镜 → 逐镜生成画面（+可选配音），API key 鉴权，返回 `202 {drama_id, status: "queued"}`。

```json
{
  "script": "雨夜，巷口。他撑着伞……",
  "style": "水墨国风",
  "shots": 8,                  // 2-24，默认 8
  "storyboard_model": "gpt-4o-mini",  // 可选：chat 模型做分镜；缺省按句子简单拆分
  "image_model": "flux-1",           // 必填：image 能力 + unit_price
  "tts_model": "cosyvoice-v1"        // 可选：tts 能力 + unit_price
}
```

- 计费：提交时冻结 `shots × (图像 unit_price + 配音 unit_price)`，**全部镜头成功才结算；任一镜头失败 → 整部失败并全额退回**（无部分计费）。分镜 LLM 调用暂不单独计费。
- 单部总超时 30 分钟；重启时残留 running 的漫剧标记失败并退回。

### GET /v1/drama/status/{drama_id}

查询漫剧进度。仅属主 key 可读。返回含 `shots` 数组（即**分镜素材包**）：

```json
{
  "drama_id": "…", "status": "running", "title": "…", "style": "水墨国风",
  "image_model": "flux-1", "tts_model": "cosyvoice-v1", "shots_planned": 8,
  "shots": [
    {"shot_no": 1, "scene": "雨夜巷口", "dialogue": "你终于来了",
     "image_prompt": "ink wash, rainy alley at night…",
     "status": "succeeded", "image_url": "https://…", "audio_url": "https://…"}
  ],
  "cost_usd": 0, "error": "", "created_at": "…", "updated_at": "…"
}
```

- 前端提供"导出素材包"：即此 JSON（shots + 元数据）下载。
- **成片（P2-1b）**：所有镜头成功后，服务端用 ffmpeg 自动合成 MP4（1280x720@30：画面信箱铺满 + 台词字幕烧录 + 配音/静音，镜头间直接拼接），完成后响应里多一个 `video_url`（`/media/dramas/{drama_id}.mp4`，公开访问，UUID 即凭证）与 `video_error`（未生成时的原因，如服务器没装 ffmpeg）。
  - 成片在**结算之后**渲染：渲染失败不影响扣费，素材包仍然有效。
  - 服务器主机需安装 `ffmpeg`/`ffprobe`（`apk add ffmpeg` / `apt install ffmpeg`）；中文字幕建议安装 CJK 字体（如 `font-noto-cjk`）。视频存于 `MODELHUB_MEDIA_DIR`（默认 `./media`）。

### POST /v1/audio/speech（P3-4 实时语音）

OpenAI 兼容的**同步**文本转语音：音频直接随 HTTP 响应返回（不走任务队列、无需轮询），客户端拿到即可播放。

```bash
curl $B/v1/audio/speech \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"cosyvoice-v1","input":"你好，世界","voice":"longxiaochun","response_format":"mp3"}' \
  -o out.mp3
```

```python
from openai import OpenAI
client = OpenAI(base_url="https://<host>/v1", api_key="sk-xxxx")
audio = client.audio.speech.create(
    model="cosyvoice-v1", voice="longxiaochun", input="你好，世界")
audio.write_to_file("out.mp3")
```

- `model` 必须是带 `tts` capability 的模型；`input` 必填、≤ 4096 字符；`voice` / `speed` 原样转发上游（OpenAI 兼容渠道）；`response_format` ∈ mp3/opus/aac/flac/wav/pcm（缺省 mp3），决定响应 `Content-Type`。
- **上游协议**：`provider=dashscope` 走 CosyVoice（返回音频 URL，网关下载后转发为字节流）；其余 provider 按 OpenAI 兼容 `POST /audio/speech` 处理（二进制响应）。
- **计费**：按模型 `unit_price` 计 1 个单位（与 `/v1/media/generate` 的 TTS 一致），调用前冻结、成功结算、失败全额解冻；组织 key 扣组织钱包（列表价）。
- 失败（上游错误/无通道/余额不足等）返回对应 4xx/502，冻结全额解冻，不落任务、不扣费。

### POST /v1/realtime（P4-1 双向实时语音）

OpenAI Realtime API（麦克风↔麦克风）的 WebSocket 透传代理。客户端走标准 OpenAI WS 协议，**鉴权在子协议头里**（不是 Bearer）：

```python
import websockets, json

uri = "wss://<host>/v1/realtime"
proto = "openai-insecure-api-key.sk-xxxx"   # ModelHub 的 API key
async with websockets.connect(uri, subprotocols=[proto]) as ws:
    await ws.send(json.dumps({
        "type": "session.update",
        "session": {"model": "gpt-realtime-1", "modalities": ["text", "audio"]},
    }))
    # 之后原样透传：input_audio_buffer.append / response.audio.delta / ...
```

- 流程：upgrade（回显 `openai-insecure-api-key.*` 子协议）→ 等待首个带 `model` 的 `session.update`（此前帧被缓冲）→ 校验模型 enabled + `realtime` capability + key 白名单 → 拨号该模型通道的 `/realtime` WS 端点 → 双向透传。
- **计费**：按会话累计 `response.done` 的用量——优先上游报告的 `usage.cost`（USD），缺失则按 `input_tokens`/`output_tokens` × 模型每 1k 单价。会话结束按实际用量结算（**无预冻结**；进程中途崩溃最多丢该会话计费）。组织 key 扣组织钱包（列表价），用量日志记 `realtime:<model>`。
- 模型需在 `capabilities` 里带 `"realtime"`（自由数组，建模型时直接加）。
- 边界：SDK 级能力，无 Web UI；WS 帧直接透传，网关不改协议；上游断开会向客户端发 `error: realtime_upstream_closed`。

## 通道故障切换（P5-1 / P6-1）

- 同一模型配置多个通道（按 `priority` 降序）时，网关自动**按优先级逐个重试**：当前通道**传输失败**（连不上/超时）或返回**可重试状态码**（401/403/408/429/500/502/503/504）时，自动换下一个通道。400/404 等请求类错误原样转发（换个通道也一样错）。
- **失败冷却 5 分钟**：失败过的通道短时间内不再尝试（避免每个请求都撞一次死通道）；任一次成功即恢复。冷却状态是**单实例内存**（与 P4-3 限流同边界），但**同步接口与任务 worker 共享同一份状态**（一个通道挂了，两边都不再撞它）。`GET /admin/channels` 的每个通道多返回 `health: "ok" | "cooldown"` 与 `cooldown_until`（RFC3339）。
- 全部通道冷却中 → 502 `no_channel`（提示稍后重试）；没有任何启用通道 → 同样 `no_channel`。
- 生效范围：
  - **同步接口（P5-1）**：`/v1/chat/completions`（同步+流式）与 `/v1/audio/speech`（含 dashscope CosyVoice）。**响应开始后**的断流不重试（客户端已有部分输出）。
  - **任务管线（P6-1）**：`/v1/media/generate`（image/video/music/tts）整任务按通道重试（共享同一个 10 分钟超时预算）；`/v1/drama/generate` 在**镜头粒度**切换——某镜头图像/配音遇可用性问题时，同一镜头换下一个通道重试，分镜 LLM 调用同样按通道重试。上游**任务内容级失败**（审核拒绝、`upstream task failed`）与超时不切换通道（换个通道也一样错）。
- 计费不受影响：冻结/结算/解冻按最终结果执行一次（成功结算，全部失败解冻），重试不重复扣费。

## 限流（P4-3）

- 环境变量 `MODELHUB_RPM` / `MODELHUB_TPM`（请求/分钟、token/分钟，按 **API key** 计数；0 或未设 = 关闭）。
- 固定 1 分钟窗口（Unix 分钟桶，非滑动窗口）：RPM 在请求进入时计数，TPM 在请求结算后按**实际** token 记账（超限在下一个请求入口拦截）。
- 超限返回 **429 `rate_limited`**（错误消息指明是 rpm 还是 tpm）。四个计费入口 + realtime 统一生效；单实例内存实现，多实例部署时限流为每实例近似值。

## 账号接口（M3）

认证模型：邮箱 + 密码（argon2id）。登录/注册返回 token 对：

- `access_token`：JWT（HS256，签名密钥派生自 `MASTER_KEY`），15 分钟有效，用于 `Authorization: Bearer`。
- `refresh_token`：30 天有效，**单次使用**，存库可吊销；刷新后旧 token 立即失效。

错误码：`invalid_credentials`、`invalid_email`、`weak_password`（< 8 位）、`email_exists`（409）、`invalid_refresh_token`、`invalid_token`（401）。

### POST /api/auth/register / /api/auth/login

```json
{"email": "alice@example.com", "password": "secret123"}
```

注册成功返回 201 + token 对；登录返回 200 + token 对：

```json
{
  "access_token": "eyJ...",
  "refresh_token": "4f1c...",
  "token_type": "Bearer",
  "access_token_expires_in": 900
}
```

注册即创建账户（余额为 0）；充值由管理员通过 `/admin/users/:id/credit` 完成。

### POST /api/auth/refresh

`{"refresh_token": "..."}` → 200 + 新 token 对（旧 refresh token 作废）。

### POST /api/auth/logout

`{"refresh_token": "..."}` → 吊销该 refresh token（幂等）。

### GET /api/models（公开，无需鉴权）

启用中的模型 + 实时价格（供定价页）：

```json
{"data": [{"id": "gpt-4o-mini", "provider": "openai", "capabilities": ["chat"], "input_price_per_1k": 0.15, "output_price_per_1k": 0.6}]}
```

### GET /api/me

当前用户：`id`、`email`、`balance_micro`、`balance_usd`、`enabled`、`created_at`；是代理时另有 `agent_rate`（批发系数）。

### GET /api/me/ledger

当前用户的余额流水（新→旧，`limit` 默认 50、上限 200，`offset` 可选）。

### GET /api/me/keys / POST /api/me/keys / DELETE /api/me/keys/:id

当前用户自己的 API key：

- `POST {"name": "dev"}` → 201 `{"key": "sk-...", "id": 1, "name": "dev"}`，**明文只返回一次**。
- 列表返回 `id`、`name`、`allowed_models`、`spend_micro`/`spend_usd`、`created_at`（无明文）。
- `DELETE` 仅能删自己的 key；删除后使用该 key 的请求立即 401。

### 分销子 Key（P2-3，仅代理账号可用；非代理 403 `not_agent`）

`GET /api/me/subkeys` / `POST /api/me/subkeys` / `DELETE /api/me/subkeys/:id`

- `POST {"name": "客户-张三", "markup": 1.2, "quota_usd": 50, "allowed_models": ["gpt-4o-mini"], "expires_at": "2027-01-01T00:00:00Z"}`（后四项可选，`markup ∈ (0, 100]`）→ 201 `{"key": "sk-...", "id": 2, "name": "客户-张三"}`，**明文只返回一次**。
- 列表返回 `id`、`name`、`markup`、`allowed_models`、`spend_micro`/`spend_usd`、`quota_usd`（如有）、`created_at`。
- 子 key 的用量从**代理余额**按代理批发价扣减；`GET /api/me/keys`（自己的 key）不含子 key，两者互不越权。

### 在线充值（P2-4）

用户用人民币充值账户余额（USD 计价）。渠道：`yipay`（开源易支付 mapi，聚合支付宝/微信）、`alipay`（支付宝官方当面付二维码）、`wechat`（微信支付官方 v3 扫码）。渠道凭据在环境变量中配置（见 server/.env.example），**全部凭据齐备的渠道才启用**；`PAY_CNY_PER_USD` 为折算汇率（如 `7.2` = 1 USD 收 7.2 CNY），未设置时充值功能关闭。

```bash
# 可用渠道与汇率（JWT 登录态）
curl $B/api/me/recharge/config -H "Authorization: Bearer <access_token>"
# → {"enabled":true,"cny_per_usd":7.2,"methods":["yipay","alipay","wechat"],"min_cny":100,"max_cny":1000000}

# 创建充值订单（金额单位：分；¥100 = 10000）
curl -X POST $B/api/me/recharges -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{"amount_cny":10000,"method":"alipay"}'
# yipay 可加 "sub_type":"alipay"|"wxpay"
# → 201 {"order":{...,"order_no":"RH...","status":"pending","credit_micro":...},
#       "payment":{"qr_code":"...","pay_url":"..."}}
#    qr_code 非空 → 前端渲染成二维码扫码支付；pay_url 非空（yipay）→ 跳转支付页

# 查询自己的订单 / 单个订单（状态轮询：pending → paid / failed）
curl $B/api/me/recharges -H "Authorization: Bearer <access_token>"
curl $B/api/me/recharges/1 -H "Authorization: Bearer <access_token>"
```

- **入账时机**：以渠道**异步回调**为准（回调先验签/验密，再入账）。下单时 `credit_micro` 已按当时汇率锁定，回调只入账这个固定值。
- **回调端点**（公网可达、无鉴权，靠渠道签名保护）：`GET|POST /pay/notify/yipay`、`POST /pay/notify/alipay`、`POST /pay/notify/wechat`。需将 `PAY_PUBLIC_URL`（或反代域名）配置为渠道可达的地址。
- **幂等**：同一订单重复回调不会重复入账（订单状态翻转 + ledger request_id 唯一约束双重保险）；回调金额与订单不符则拒绝。
- 单笔限额 ¥1 – ¥10000（`min_cny`/`max_cny` 分）。

## 组织（P3-3 多租户）

组织 = 租户：独立钱包（`balance`，micro-USD）+ 成员（owner/admin/member）+ 组织 API key。
组织 key 的 `user_id` 为空、`org_id` 非空，所有调用（chat / media / drama）**按列表价**扣费组织钱包（无代理折扣）。
组织充值目前**仅管理员手动**（见下文管理接口），不做组织在线支付。

角色规则：

- owner：创建者，唯一；不可改角色、不可被移除。
- admin：可管理成员与 key；不可改 owner。
- member：只能查看组织信息、成员列表与 key 列表；管理操作 403。
- 非成员访问他人组织一律 404（不泄露组织是否存在）。

```bash
# 我加入的组织列表（含本人角色 / 组织余额）
curl $B/api/me/orgs -H "Authorization: Bearer <access_token>"

# 创建组织（本人自动成为 owner）
curl $B/api/me/orgs -X POST -H "Authorization: Bearer <access_token>" -d '{"name":"acme"}'

# 组织详情（含成员列表；仅需成员身份）
curl $B/api/me/orgs/1 -H "Authorization: Bearer <access_token>"

# 邀请成员（admin+；email 必须已注册，重复 409 member_exists；role: admin|member，缺省 member）
curl $B/api/me/orgs/1/members -X POST -H "Authorization: Bearer <access_token>" -d '{"email":"dev@acme.io","role":"admin"}'

# 改成员角色（admin+；owner 不可改 → 400）
curl $B/api/me/orgs/1/members/2 -X PUT -H "Authorization: Bearer <access_token>" -d '{"role":"member"}'

# 移除成员（admin+；本人也可移除自己；owner 不可移除 → 400）
curl $B/api/me/orgs/1/members/2 -X DELETE -H "Authorization: Bearer <access_token>"

# 组织 key 列表（任何成员）
curl $B/api/me/orgs/1/keys -H "Authorization: Bearer <access_token>"

# 创建组织 key（admin+；明文 key 仅返回这一次，之后只能看到名称与用量）
curl $B/api/me/orgs/1/keys -X POST -H "Authorization: Bearer <access_token>" \
  -d '{"name":"acme-prod","quota_usd":100}'
# → 201 {"key":"sk-...","org":{"id":7,"name":"acme-prod",...}}

# 删除组织 key（admin+）
curl $B/api/me/orgs/1/keys/7 -X DELETE -H "Authorization: Bearer <access_token>"

# 组织用量（admin+：汇总 + 最近明细，limit 上限 200）
curl "$B/api/me/orgs/1/usage?limit=50" -H "Authorization: Bearer <access_token>"

# 组织钱包流水（admin+；kind: credit / hold / release / debit）
curl "$B/api/me/orgs/1/ledger?limit=50" -H "Authorization: Bearer <access_token>"
```

计费路由：请求带组织 key 时，hold/settle/release 全部落在组织钱包与 `org_ledger_entries`；
媒体/漫剧等异步任务由 worker 通过 `api_keys.org_id` 解析归属，成功后结算、失败全额解冻。
个人 key 行为不变（扣个人余额、代理批发价照旧）。

## 管理接口

### POST /admin/models

```json
{
  "model_id": "gpt-4o-mini",
  "provider": "openai",
  "upstream_model": "gpt-4o-mini",
  "capabilities": ["chat"],
  "input_price_per_1k": 0.15,
  "output_price_per_1k": 0.6
}
```

### POST /admin/channels

```json
{
  "name": "openai-main",
  "provider": "openai",
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model_id": "gpt-4o-mini",
  "priority": 100
}
```

### POST /admin/api-keys

`{"name": "dev"}` → 返回 `{"key": "sk-...", "id": 1}`，**明文只返回一次**。

### PATCH /admin/api-keys/:id

修改已有 key，所有字段可选；不传即不变：

```json
{
  "name": "dev",
  "user_id": 1,
  "quota_usd": 100,
  "allowed_models": ["gpt-4o-mini"],
  "expires_at": "2027-01-01T00:00:00Z"
}
```

- `user_id: 0` 解除与用户的绑定；`user_id > 0` 绑定到该用户（用户必须存在）。
- `quota_usd: 0` 清除额度。
- `allowed_models: []` 表示允许全部模型。
- `expires_at: ""` 清除过期时间，否则为 RFC3339。

### GET /admin/api-keys / DELETE /admin/api-keys/:id

### 用户与充值

```bash
# 创建用户（可带初始余额）
curl -X POST $B/admin/users -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"email":"alice@example.com","initial_balance_usd":100}'

# 手动充值
curl -X POST $B/admin/users/1/credit -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"amount_usd":50,"reason":"月度充值"}'

# 用户列表 / 单个用户
curl $B/admin/users -H "Authorization: Bearer $MASTER_KEY"
curl $B/admin/users/1 -H "Authorization: Bearer $MASTER_KEY"

# 账本流水（新→旧，limit 上限 200）
curl "$B/admin/users/1/ledger?limit=50&offset=0" -H "Authorization: Bearer $MASTER_KEY"
```

返回中 `balance_micro` 为精确值（微美元），`balance_usd` 为展示值。

### 代理分销（P2-3）

```bash
# 标记用户为代理（rate ∈ (0,1]，如 0.85 = 八五折批发）
curl -X POST $B/admin/agents -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"user_id":1,"rate":0.85}'

# 代理列表（含 email）
curl $B/admin/agents -H "Authorization: Bearer $MASTER_KEY"

# 修改批发系数 / 撤销代理（其子 key 降级为普通 key，不删除）
curl -X PUT $B/admin/agents/1 -H "Authorization: Bearer $MASTER_KEY" -d '{"rate":0.8}'
curl -X DELETE $B/admin/agents/1 -H "Authorization: Bearer $MASTER_KEY"
```

- 已是代理的用户重复创建 → 409 `already_agent`；`rate` 越界 → 400 `invalid_rate`。
- 效果：该用户后续所有 hold/settle 按批发价执行（历史流水不受影响）；`/admin/users` 列表与 `GET /api/me` 中带 `agent_rate`。
- 代理在 Web 控制台（或 `/api/me/subkeys`）自助创建子 key 分发给客户。

### 智能体模板（P2-2）

```bash
# 模板列表（含 system_prompt / tools / enabled / created_at）
curl $B/admin/assistants -H "Authorization: Bearer $MASTER_KEY"

# 新建模板（agent_id 全局唯一；model 必须是已存在的模型；tools 为 OpenAI 格式函数定义数组，可选）
curl -X POST $B/admin/assistants -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"agent_id":"agent-poet","name":"诗人","description":"写诗","system_prompt":"你是一位诗人……","model":"gpt-4o-mini","tools":[{"type":"function","function":{"name":"get_weather","description":"查询天气","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]}'

# 局部更新（改绑模型 / 改 prompt / 改 tools / 启停；tools 传 [] 清空）
curl -X PATCH $B/admin/assistants/1 -H "Authorization: Bearer $MASTER_KEY" -d '{"model":"gpt-4o"}'
curl -X PATCH $B/admin/assistants/1 -H "Authorization: Bearer $MASTER_KEY" -d '{"tools":[]}'
curl -X PATCH $B/admin/assistants/1 -H "Authorization: Bearer $MASTER_KEY" -d '{"enabled":false}'

# 删除
curl -X DELETE $B/admin/assistants/1 -H "Authorization: Bearer $MASTER_KEY"
```

- `agent_id` 重复 → 409 `agent_exists`；改绑不存在的模型 → 400；`tools` 非数组或元素缺 `function` 对象 → 400 `invalid_tools`。
- 停用的模板不出现在 `GET /v1/agents`，且以其 id 发起 chat 会 404。

### 支付配置（P3-2，渠道凭据后台热配置）

渠道凭据**运行时可改**，存 `pay_settings` / `pay_channels` 表。env 里的 `PAY_*` 只是**首次启动的初始默认**（播种一次，之后 env 改动不生效）；播种后管理端是唯一事实来源，保存即热加载、无需重启。

```bash
# 查看当前配置（敏感字段返回 ********；每渠道带 status.ok 表示凭据能否成功构建）
curl $B/admin/pay-config -H "Authorization: Bearer $MASTER_KEY"

# 改汇率 / 公网地址（0 = 关闭充值）
curl -X PUT $B/admin/pay-config -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"cny_per_usd":7.2,"public_url":"https://api.example.com"}'

# 配置易支付并启用（enabled 缺省保持原值；敏感字段回传 ******** 或空 = 保持不变，传新值 = 替换）
curl -X PUT $B/admin/pay-config -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"channels":{"yipay":{"enabled":true,"config":{"mapi_url":"https://pay.example.com/mapi.php","pid":"1001","key":"***"}}}}'

# 配置支付宝（private_key / public_key 支持 PEM 文本，多行直接放 JSON 字符串）
curl -X PUT $B/admin/pay-config -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"channels":{"alipay":{"enabled":true,"config":{"app_id":"2021...","private_key":"***\n...","public_key":"***\n..."}}}}'

# 停用某渠道（保留已存凭据）
curl -X PUT $B/admin/pay-config -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"channels":{"wechat":{"enabled":false}}}'
```

- 渠道字段：`yipay: mapi_url / pid / key`；`alipay: app_id / private_key / public_key`；`wechat: mch_id / app_id / api_v3_key / merchant_serial / private_key / platform_key`（app_id 对微信 Native 可空）。
- 凭据缺失或非法（如 RSA 解析失败）的渠道**不启用**，并在 `status.error` 说明原因，不影响其他渠道与服务器运行。
- PUT 响应与 GET 同形（脱敏后的最新配置 + status），一次往返即可刷新界面。
- `cny_per_usd = 0` 时所有渠道一律不启用（`status.error = "recharge disabled"`）。

### 组织（P3-3）

```bash
# 全平台组织（新→旧，含 owner email 与成员数，limit 上限 200）
curl "$B/admin/organizations?limit=50" -H "Authorization: Bearer $MASTER_KEY"

# 组织钱包手动充值（唯一组织充值方式；amount_usd > 0，reason 缺省 "admin credit"）
curl -X POST $B/admin/organizations/1/credit -H "Authorization: Bearer $MASTER_KEY" \
  -d '{"amount_usd":50,"reason":"annual plan"}'
```

组织没有删除接口（避免误删钱包与流水）；停用某个组织可由管理端删除其全部 key 实现（其请求将立即 401）。

### 充值订单（P2-4）

```bash
# 全平台充值订单（新→旧，含 email，limit 上限 200）
curl "$B/admin/recharges?limit=50" -H "Authorization: Bearer $MASTER_KEY"
```

返回 `id`、`user_id`、`email`、`order_no`、`method`、`amount_cny`/`amount_yuan`、`credit_micro`/`credit_usd`、`status`（pending/paid/failed）、`created_at`、`paid_at`。`failed` = 渠道下单失败（用户可重新下单）；`pending` 超期未付的订单无需处理（不会入账）。

### 用量统计

```bash
# 最近调用明细（可按 key 过滤，limit 上限 500）
curl "$B/admin/usage?api_key_id=1&limit=50" -H "Authorization: Bearer $MASTER_KEY"

# 汇总（可按 key 过滤、按时间起点过滤，RFC3339）
curl "$B/admin/usage/summary?api_key_id=1&since=2026-09-01T00:00:00Z" -H "Authorization: Bearer $MASTER_KEY"
```

汇总返回：`{"requests":n,"prompt_tokens":n,"completion_tokens":n,"cost_micro":n,"cost_usd":x}`。

### GET /admin/models / DELETE /admin/models/:id / POST /admin/models

创建模型时的价格字段：

- `input_price_per_1k` / `output_price_per_1k`：token 计价（chat 模型用）。
- `price_unit`：`token`（默认）或 `image` / `video` / `music` / `tts`。
- `unit_price`：每件 USD（`price_unit != token` 时必填，如一张图 $0.02、一条视频 $0.3）。

### GET /admin/channels / DELETE /admin/channels/:id

### 媒体任务（M4）

```bash
# 最近任务（新→旧，可按 key 过滤，limit 上限 200）
curl "$B/admin/tasks?api_key_id=1&limit=50" -H "Authorization: Bearer $MASTER_KEY"
```

返回结构与 `GET /v1/media/status/{task_id}` 相同（列表包在 `data` 里）。

### 漫剧（P2-1）

```bash
# 最近漫剧（新→旧，可按 key 过滤，limit 上限 200）
curl "$B/admin/dramas?api_key_id=1&limit=50" -H "Authorization: Bearer $MASTER_KEY"
```

返回结构与 `GET /v1/drama/status/{drama_id}` 相同（列表包在 `data` 里）。