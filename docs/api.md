# API 参考

## 鉴权

- 开放接口（`/v1/*`）：`Authorization: Bearer sk-xxxx`（API key）
- 管理接口（`/admin/*`）：`Authorization: Bearer $MASTER_KEY`

## 开放接口

### GET /v1/models

返回启用的模型列表（OpenAI 兼容格式）。

### POST /v1/chat/completions

OpenAI 兼容。`stream: true` 时返回 SSE。请求体其余字段原样透传上游。

```json
{
  "model": "gpt-4o-mini",
  "messages": [{"role": "user", "content": "hi"}],
  "stream": false
}
```

错误：401（key 无效/过期）、404（model 不存在或无可用通道）。

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

`{"name": "dev"}` → 返回 `{"key": "sk-..."}`，**明文只返回一次**。

### GET /admin/api-keys / DELETE /admin/api-keys/:id

### GET /admin/models / DELETE /admin/models/:id

### GET /admin/channels / DELETE /admin/channels/:id