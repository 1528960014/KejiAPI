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

## 开放接口

### GET /v1/models

返回启用的模型列表（OpenAI 兼容格式）。

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
  - `music`：**暂无适配器**，提交会排队但任务以明确错误失败并退回冻结
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

- 前端提供"导出素材包"：即此 JSON（shots + 元数据）下载。MP4 成片合成为后续里程碑。

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

当前用户：`id`、`email`、`balance_micro`、`balance_usd`、`enabled`、`created_at`。

### GET /api/me/ledger

当前用户的余额流水（新→旧，`limit` 默认 50、上限 200，`offset` 可选）。

### GET /api/me/keys / POST /api/me/keys / DELETE /api/me/keys/:id

当前用户自己的 API key：

- `POST {"name": "dev"}` → 201 `{"key": "sk-...", "id": 1, "name": "dev"}`，**明文只返回一次**。
- 列表返回 `id`、`name`、`allowed_models`、`spend_micro`/`spend_usd`、`created_at`（无明文）。
- `DELETE` 仅能删自己的 key；删除后使用该 key 的请求立即 401。

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