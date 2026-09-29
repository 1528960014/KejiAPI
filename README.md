# ModelHub

开源、可自托管的多模型 AI 网关与平台（BYO upstream keys）。统一 OpenAI 兼容 API，把上游各家模型（OpenAI / Anthropic / Gemini / 通义 / SiliconFlow 等）聚合成一套接口、一套计费、一套控制台。

> This is an original open-source project. It shares no code, assets, or design with any other commercial product.

## 特性（v1）

- **模型网关**：OpenAI 兼容 `/v1/chat/completions`（SSE 流式）、`/v1/models`；通道（channel）级多上游凭据、优先级与故障切换
- **异步任务管线**（M4）：`/v1/media/generate` 图像/视频/TTS 生成任务（DB 队列 + worker）、按件计价与冻结/结算、状态轮询；音乐适配器规划中
- **漫剧工坊**（P2-1）：`/v1/drama/generate` 剧本 → LLM 分镜 → 逐镜图+配音，all-or-nothing 计费；分镜素材包 JSON 导出 + ffmpeg 自动合成 MP4 成片（画面+字幕+配音）
- **计费**：账本式余额（预估 → 冻结 → 结算）、API Key 管理（sk- 前缀，仅存哈希）、用量统计
- **Web 控制台**：多模型对比聊天、生成工作台、API 控制台、中英双语、深浅主题
- **部署简单**：`docker compose up -d` 起依赖（PostgreSQL / Redis / MinIO），服务端单二进制

## 快速开始

前置：Go 1.24+、Docker、pnpm（仅前端开发需要）。漫剧 MP4 成片需要主机安装 `ffmpeg`/`ffprobe`（`apk add ffmpeg` / `apt install ffmpeg`，中文字幕建议另装 CJK 字体）。

```bash
# 1. 启动依赖服务
cd deploy
docker compose up -d

# 2. 配置并启动服务端
cp .env.example .env   # 修改 MASTER_KEY
cd server
go mod tidy
go run ./cmd/modelhub

# 3. 初始化数据（通过管理 API，MASTER_KEY 鉴权）
# 添加模型
curl -X POST http://127.0.0.1:8080/admin/models \
  -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" \
  -d '{"model_id":"gpt-4o-mini","provider":"openai","upstream_model":"gpt-4o-mini","input_price_per_1k":0.15,"output_price_per_1k":0.6}'
# 添加通道（你的上游 API key）
curl -X POST http://127.0.0.1:8080/admin/channels \
  -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" \
  -d '{"name":"openai-main","provider":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-...","model_id":"gpt-4o-mini"}'
# 创建 API key（返回值仅显示一次）
curl -X POST http://127.0.0.1:8080/admin/api-keys -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" -d '{"name":"dev"}'

# 4. 调用（OpenAI 兼容）
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":false}'
```

## 目录结构

```
modelhub/
├── server/     # Go 网关：internal/{api,auth,gateway,billing,store,config}
├── web/        # Vue 3 + Vite + Element Plus 控制台
├── deploy/     # docker-compose（依赖服务）+ nginx.conf（生产）
└── docs/       # 架构与 API 文档
```

## 文档

- [架构设计](docs/architecture.md)
- [API 参考](docs/api.md)

## 路线图

- [x] M1 网关核心（OpenAI 兼容聊天 / 流式 / API key）
- [x] M2 计费账本（余额冻结与结算、用量统计、管理端手动充值）
- [x] M3 Web 控制台（多模型聊天、API 控制台、i18n、主题）
- [x] M4 任务管线（图像 → 视频 / 音乐 / TTS；图像/视频/TTS 已落地，音乐适配器见 docs/api.md）
- [x] P2-1a 漫剧管线（分镜 + 逐镜素材包 + JSON 导出）
- [x] P2-1b 漫剧成片（ffmpeg 合成 MP4：画面+字幕+配音）
- [ ] 二期：智能体、代理分销、在线支付、更多语言

## 合规说明

- 本项目不提供任何模型推理能力；请自备上游 API key，并遵守对应服务商的使用条款（部分服务商限制转售）。
- 请勿在 `.env` 或代码库中提交真实密钥。

## 贡献

参见 [CONTRIBUTING.md](CONTRIBUTING.md)。欢迎 PR 与 issue。

## 许可证

[MIT](LICENSE)