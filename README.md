# ModelHub

开源、可自托管的多模型 AI 网关与平台（BYO upstream keys）。统一 OpenAI 兼容 API，把上游各家模型（OpenAI / Anthropic / Gemini / 通义 / SiliconFlow 等）聚合成一套接口、一套计费、一套控制台。

> This is an original open-source project. It shares no code, assets, or design with any other commercial product.

## 特性（v1）

- **模型网关**：OpenAI 兼容 `/v1/chat/completions`（SSE 流式）、`/v1/models`；通道（channel）级多上游凭据、优先级与故障切换
- **异步任务管线**（M4）：`/v1/media/generate` 图像/视频/TTS 生成任务（DB 队列 + worker）、按件计价与冻结/结算、状态轮询；音乐适配器规划中
- **漫剧工坊**（P2-1）：`/v1/drama/generate` 剧本 → LLM 分镜 → 逐镜图+配音，all-or-nothing 计费；分镜素材包 JSON 导出 + ffmpeg 自动合成 MP4 成片（画面+字幕+配音）
- **代理分销**（P2-3）：代理用户按批发系数结算全部消费，控制台自助创建/管理分销子 Key（独立白名单/额度/有效期 + 加价倍率展示）
- **智能体**（P2-2）：预定义 agent 模板（固定 system prompt + 绑定模型），`model: agent_id` 直接调用、按真实模型计费；内置翻译官/写手/客服，聊天页可选、管理端可编辑
- **在线支付**（P2-4）：人民币充值 → USD 余额；支持开源易支付（聚合）/ 支付宝官方（当面付）/ 微信支付官方（v3 扫码），回调验签、下单锁价、幂等入账
- **计费**：账本式余额（预估 → 冻结 → 结算）、API Key 管理（sk- 前缀，仅存哈希）、用量统计
- **Web 控制台**：多模型对比聊天、生成工作台、API 控制台、6 语言（中/英/日/韩/俄/西）、深浅主题
- **部署简单**：`docker compose up -d` 起依赖（PostgreSQL / Redis / MinIO），服务端单二进制

## 快速开始

前置：Go 1.24+、Docker、pnpm（仅前端开发需要）。漫剧 MP4 成片需要主机安装 `ffmpeg`/`ffprobe`（`apk add ffmpeg` / `apt install ffmpeg`，中文字幕建议另装 CJK 字体）。

```bash
# 1. 启动依赖服务
cd deploy
docker compose up -d

# 2. 配置并启动服务端
cp server/.env.example server/.env   # 修改 MASTER_KEY；在线充值可选配 PAY_*
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

## 生产部署（Docker 全套）

`deploy/` 提供完整编排（app + web + postgres + redis + minio），一条命令起站：

```bash
git clone https://github.com/1528960014/modelhub /www/modelhub && cd /www/modelhub
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env：MASTER_KEY（openssl rand -hex 32）、PAY_PUBLIC_URL 等
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
curl http://127.0.0.1/healthz
```

- 入口：`web` 容器占 80 端口（SPA + `/v1`、`/admin` 反代到 `app`）；`app` 直连 8080 可留作调试。
- 数据都在命名卷（`pgdata` / `mediadata` / `miniodata`）；PostgreSQL/Redis/MinIO 只绑 `127.0.0.1`。
- 漫剧合成需要镜像内 ffmpeg + Noto CJK 字体（`Dockerfile.server` 已含）。
- 防火墙/安全组放行 80（如需 443 用面板或 certbot 再套一层 TLS）。

## 目录结构

```
modelhub/
├── server/     # Go 网关：internal/{api,auth,gateway,billing,store,config}
├── web/        # Vue 3 + Vite + Element Plus 控制台
├── deploy/     # 生产编排：Dockerfile(server/web) + docker-compose + nginx 模板
└── docs/       # 架构与 API 文档
```

## 文档

- [架构设计](docs/architecture.md)
- [API 参考](docs/api.md)

## 路线图

- [x] M1 网关核心（OpenAI 兼容聊天 / 流式 / API key）
- [x] M2 计费账本（余额冻结与结算、用量统计、管理端手动充值）
- [x] M3 Web 控制台（多模型聊天、API 控制台、i18n、主题）
- [x] M4 任务管线（图像 → 视频 / 音乐 / TTS，适配器全部落地；音乐 = suno 社区 API）
- [x] P2-1a 漫剧管线（分镜 + 逐镜素材包 + JSON 导出）
- [x] P2-1b 漫剧成片（ffmpeg 合成 MP4：画面+字幕+配音）
- [x] P2-2 智能体（预定义 agent 模板，model=agent_id + system prompt 注入）
- [x] P2-3 代理分销（批发价结算 + 分销子 Key）
- [x] P2-5 更多语言（ja/ko/ru/es，共 6 语言切换）
- [x] P2-4 在线支付（易支付 / 支付宝官方 / 微信官方，人民币 → USD 余额）
- [x] P3-1 智能体工具调用（模板 tools 注入，OpenAI function calling，客户端执行工具）
- [x] P3-2 支付渠道后台配置（/admin/pay-config 热更新，env 仅首次启动默认）
- [x] P3-3 多租户（组织：独立钱包 + 成员 owner/admin/member + 组织 Key 按列表价计费）
- [x] P3-4 实时语音（OpenAI 兼容 POST /v1/audio/speech 同步 TTS，聊天页朗读按钮）
- [x] P4-1 双向实时语音（OpenAI Realtime WS 透传，`/v1/realtime` 子协议鉴权，会话用量结算）
- [x] P4-2 音乐适配器（suno 社区 API，`/v1/media/generate` type=music）
- [x] P4-3 每 key 限流（`MODELHUB_RPM`/`MODELHUB_TPM`，429 `rate_limited`）
- [x] P5-1 通道自动故障切换（多通道按优先级重试 + 5 分钟失败冷却，chat/语音，`/admin/channels` 健康状态）
- [x] P6-1 任务管线故障切换（媒体任务整任务重试、漫剧镜头级切换、分镜重试，与同步接口共享冷却状态）
- [ ] 后续：双向实时语音（麦克风↔麦克风，OpenAI Realtime 类协议）

## 合规说明

- 本项目不提供任何模型推理能力；请自备上游 API key，并遵守对应服务商的使用条款（部分服务商限制转售）。
- 请勿在 `.env` 或代码库中提交真实密钥。

## 贡献

参见 [CONTRIBUTING.md](CONTRIBUTING.md)。欢迎 PR 与 issue。

## 许可证

[MIT](LICENSE)