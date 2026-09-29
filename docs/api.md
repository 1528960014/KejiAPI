# API 参考

## 鉴权

- 开放接口（`/v1/*`）：`Authorization: Bearer sk-xxxx`（API key）
- 管理接口（`/admin/*`）：`Authorization: Bearer $MASTER_KEY`

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

异步生成任务（image/video/music/tts），返回 `task_id`；用 `GET /v1/media/status/{task_id}` 轮询。

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

### GET /admin/models / DELETE /admin/models/:id

### GET /admin/channels / DELETE /admin/channels/:id