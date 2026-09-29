<div align="center">

# ModelHub

**模型、应用与智能体一体的自托管 AI 网关平台**

<p align="center">
  <a href="https://github.com/1528960014/modelhub/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/1528960014/modelhub?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://go.dev/">
    <img src="https://img.shields.io/badge/go-1.24-blue" alt="go">
  </a><!--
  --><a href="https://vuejs.org/">
    <img src="https://img.shields.io/badge/frontend-Vue%203-brightgreen" alt="vue">
  </a>
  <a href="https://docs.docker.com/compose/">
    <img src="https://img.shields.io/badge/docker-compose-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#项目简介">项目简介</a> •
  <a href="#核心能力">核心能力</a> •
  <a href="#接口一览">接口一览</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#生产部署">生产部署</a> •
  <a href="#开发与目录结构">开发</a> •
  <a href="#路线图">路线图</a>
</p>

</div>

---

## 📝 项目简介

ModelHub 是一个开源、可自托管的多模型 AI 网关与运营平台（BYO upstream keys，自带上游密钥）。它把各家上游模型服务（OpenAI / Anthropic / Gemini / 通义 / SiliconFlow 等）聚合成**一套 OpenAI 兼容接口、一套账本计费、一套 Web 控制台**，并提供在线支付充值、漫剧视频管线、智能体模板、代理分销与多租户组织等运营能力。

适用场景：

- **团队共享**：把授权的上游模型访问权分发给团队成员，每人独立 API Key、独立额度与用量。
- **私有多模型服务**：一套控制台管理多个上游供应商，按优先级自动故障切换，客户端无需感知切换过程。
- **模型运营/转售**：人民币在线充值（易支付 / 支付宝 / 微信）→ USD 余额，账本式冻结结算，代理商批发系数分销，组织级钱包多租户。
- **内容生产线**：图像 → 视频 / 音乐 / TTS 异步任务管线；剧本 → LLM 分镜 → 逐镜画面+配音 → ffmpeg 合成 MP4 成片的漫剧工坊。

> [!IMPORTANT]
> - 本项目**不提供任何模型推理能力**，请自行合法获取上游 API Key、账号与模型服务，并遵守对应服务商的使用条款（部分服务商限制转售）。
> - 对外提供生成式 AI 服务时，请遵守所在司法辖区的备案、内容安全、日志留存、税务等监管要求。
> - 请勿在 `.env` 或代码库中提交真实密钥。

> [!WARNING]
> 将本项目作为公开 API 转售服务运营前，请先完成所在辖区要求的全部备案、支付、内容安全与上游授权义务。

---

<a id="核心能力"></a>

## 核心能力

| 能力 | 说明 |
| --- | --- |
| **模型网关** | OpenAI 兼容 `/v1/chat/completions`（SSE 流式）与 `/v1/models`；通道（channel）级多上游凭据、优先级与权重、**自动故障切换**（失败冷却 5 分钟，`/admin/channels` 可见健康状态） |
| **计费账本** | 预估 → 冻结 → 结算的账本式余额；按输入/输出 token 计价；每 Key 可选 RPM/TPM 限流（429 `rate_limited`）；用量统计与汇总 |
| **异步任务管线** | `/v1/media/generate`：图像 / 视频 / 音乐（suno 社区 API）/ TTS 生成任务，DB 队列 + worker、按件计价冻结/结算、状态轮询；**任务级故障切换**（整任务重试 + 镜头级切换） |
| **漫剧工坊** | `/v1/drama/generate`：剧本 → LLM 分镜 → 逐镜画面+配音，all-or-nothing 计费；分镜素材包 JSON 导出；ffmpeg 自动合成带字幕、配音的 MP4 成片 |
| **智能体** | 预定义 agent 模板（固定 system prompt + 绑定模型 + OpenAI function calling 工具注入）；`model: agent_id` 直接调用、按真实模型计费；内置翻译官/写手/客服，聊天页可选、管理端可编辑 |
| **实时语音** | OpenAI 兼容 `POST /v1/audio/speech` 同步 TTS（聊天页朗读按钮）；`/v1/realtime` 双向实时语音 WebSocket 透传（麦克风↔麦克风），子协议鉴权、会话级用量结算 |
| **在线支付** | 人民币充值 → USD 余额；三渠道：**开源易支付（聚合）/ 支付宝官方（当面付）/ 微信官方（v3 扫码）**；回调验签、下单锁价、幂等入账；渠道凭据**后台热配置**（env 仅作首次启动默认） |
| **代理分销** | 代理用户按批发系数结算全部消费；控制台自助创建/管理分销子 Key（独立白名单 / 额度 / 有效期 + 加价倍率展示） |
| **多租户组织** | 组织独立钱包 + 成员角色（owner / admin / member）；组织 Key 按列表价计费；组织用量与流水审计 |
| **Web 控制台** | 多模型对比聊天（工具调用 / 智能体 / 朗读 / 实时语音）、生成工作台、API 控制台、自助充值；**6 语言**（中/英/日/韩/俄/西）、深浅主题 |
| **管理后台** | 现代化侧边栏仪表盘：统计卡 + 近 7 天成本图表 + 通道健康 + 最近请求、模型 / 通道 / 用户 / Key / 智能体 / 组织 / 支付 / 用量 / 充值订单全量管理，客户端搜索与弹窗编辑 |

---

<a id="接口一览"></a>

## 接口一览

| 接口 | 常用端点 |
| --- | --- |
| OpenAI 兼容 | `POST /v1/chat/completions`（SSE）· `GET /v1/models` · `GET /v1/agents` |
| 实时语音 | `POST /v1/audio/speech` · `POST /v1/realtime`（WebSocket 升级） |
| 异步任务 | `POST /v1/media/generate` · `GET /v1/media/status/:task_id` |
| 漫剧 | `POST /v1/drama/generate` · `GET /v1/drama/status/:drama_id` · `GET /media/dramas/:uuid`（MP4） |
| 用户自助 | `POST /api/auth/{register,login,refresh,logout}` · `GET /api/me` · `/api/me/keys` · `/api/me/subkeys` · `/api/me/orgs/**` · `/api/me/recharges` |
| 管理 API（MASTER_KEY） | `/admin/models` · `/admin/channels` · `/admin/api-keys` · `/admin/users`（含 `/:id/credit`、`/:id/ledger`）· `/admin/agents` · `/admin/assistants` · `/admin/organizations` · `/admin/pay-config` · `/admin/usage`（含 `/summary`）· `/admin/tasks` · `/admin/dramas` · `/admin/recharges` |
| 支付回调 | `GET/POST /pay/notify/{yipay,alipay,wechat}` |

---

<a id="快速开始"></a>

## 快速开始

前置：Go 1.24+、Docker、pnpm（仅前端开发需要）。漫剧 MP4 成片需要主机安装 `ffmpeg`/`ffprobe`（`apt install ffmpeg`，中文字幕建议另装 CJK 字体）。

```bash
# 1. 启动依赖服务（PostgreSQL / Redis / MinIO）
cd deploy
docker compose up -d

# 2. 配置并启动服务端
cp server/.env.example server/.env   # 修改 MASTER_KEY；在线充值可选配 PAY_*
cd server
go mod tidy
go run ./cmd/modelhub

# 3. 初始化数据（管理 API，MASTER_KEY 鉴权）
curl -X POST http://127.0.0.1:8080/admin/models \
  -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" \
  -d '{"model_id":"gpt-4o-mini","provider":"openai","upstream_model":"gpt-4o-mini","input_price_per_1k":0.15,"output_price_per_1k":0.6}'
curl -X POST http://127.0.0.1:8080/admin/channels \
  -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" \
  -d '{"name":"openai-main","provider":"openai","base_url":"https://api.openai.com/v1","api_key":"sk-...","model_id":"gpt-4o-mini"}'
curl -X POST http://127.0.0.1:8080/admin/api-keys \
  -H "Authorization: Bearer $MASTER_KEY" -H "Content-Type: application/json" -d '{"name":"dev"}'

# 4. 调用（OpenAI 兼容）
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-xxxx" -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":false}'
```

打开 `http://127.0.0.1:8080`（Web 控制台）或前端开发服务器，注册账号即可使用；管理后台粘贴 `MASTER_KEY` 解锁。

<a id="生产部署"></a>

## 生产部署

`deploy/` 提供完整编排（app + web + postgres + redis + minio），一条命令起站：

```bash
git clone https://github.com/1528960014/modelhub /www/modelhub && cd /www/modelhub
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env：MASTER_KEY（openssl rand -hex 32）、PAY_PUBLIC_URL 等
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
curl http://127.0.0.1/healthz
```

| 项目 | 说明 |
| --- | --- |
| 入口 | `web` 容器默认占 80 端口（SPA + `/v1`、`/admin` 反代到 `app`）；`app` 直连 8080 可留作调试 |
| 端口冲突 | 宿主机 80 被占用（如宝塔 nginx）：`./deploy/deploy.sh host80`（web 改映射 8000），再在 Web 服务器加反代 → `http://127.0.0.1:8000` |
| 一键脚本 | `deploy/deploy.sh`：校验 Docker、生成 `.env` 提示、构建启动、健康检查 |
| 数据 | 命名卷（`pgdata` / `mediadata` / `miniodata`）；PostgreSQL / Redis / MinIO 只绑 `127.0.0.1` |
| 漫剧合成 | 镜像内置 ffmpeg + Noto CJK 字体 |
| TLS | 443 用面板或 certbot 再套一层 |

### 主要环境变量

| 变量 | 用途 |
| --- | --- |
| `MASTER_KEY` | 管理 API 与 Web 管理登录密钥，≥16 位，建议 `openssl rand -hex 32` |
| `PAY_PUBLIC_URL` | 公网地址（支付回调 / 回跳 URL 基于它拼接） |
| `MODELHUB_RPM` / `MODELHUB_TPM` | 每 Key 限流，0 = 关闭 |
| `PAY_CNY_PER_USD` | 充值汇率（0 = 关闭在线充值） |
| `PAY_YIPAY_*` | 易支付（聚合）：`MAPI_URL` / `PID` / `KEY` |
| `PAY_ALIPAY_*` | 支付宝官方：`APP_ID` / `PRIVATE_KEY` / `PUBLIC_KEY` |
| `PAY_WECHAT_*` | 微信官方：`MCH_ID` / `APP_ID` / `APIV3_KEY` / `MERCHANT_SERIAL` / `PRIVATE_KEY` / `PLATFORM_KEY` |

支付渠道凭据也可在管理后台「支付配置」页热更新（env 仅作首次启动默认值），详见 [docs/api.md](docs/api.md)。

<a id="开发与目录结构"></a>

## 开发与目录结构

后端为 Go 单二进制（内嵌前端产物），前端为 Vue 3 + Vite + Element Plus + vue-i18n：

```bash
# 前端开发（代理 /api、/v1、/admin 到 8080）
cd web && pnpm install && pnpm dev

# 后端构建（前端先 build，server 内嵌 web/dist）
cd web && pnpm build
cd ../server && go build ./cmd/modelhub
```

| 目录 | 职责 |
| --- | --- |
| `server/internal/api` | HTTP 路由与处理器（v1 / admin / api / pay） |
| `server/internal/gateway` | 上游适配、流式转发、通道故障切换 |
| `server/internal/billing` | 账本（冻结/结算）、计价、限流 |
| `server/internal/store` | PostgreSQL 持久层 |
| `server/internal/auth` | 用户 / 会话 / API Key（仅存哈希） |
| `web/` | Web 控制台与管理员后台（6 语言 i18n） |
| `deploy/` | 生产编排：Dockerfile（server/web）+ docker-compose + nginx 模板 + 一键脚本 |
| `docs/` | [架构设计](docs/architecture.md) 与 [API 参考](docs/api.md) |

## 路线图

- [x] M1 网关核心（OpenAI 兼容聊天 / 流式 / API key）
- [x] M2 计费账本（余额冻结与结算、用量统计、管理端手动充值）
- [x] M3 Web 控制台（多模型聊天、API 控制台、i18n、主题）
- [x] M4 任务管线（图像 → 视频 / 音乐 / TTS，适配器全部落地）
- [x] P2-1 漫剧管线（分镜 + 素材包 + ffmpeg MP4 成片）
- [x] P2-2 智能体（agent 模板 + 工具调用）
- [x] P2-3 代理分销（批发价结算 + 分销子 Key）
- [x] P2-4 在线支付（易支付 / 支付宝官方 / 微信官方）
- [x] P2-5 更多语言（ja/ko/ru/es，共 6 语言）
- [x] P3-1 智能体工具调用（OpenAI function calling）
- [x] P3-2 支付渠道后台热配置
- [x] P3-3 多租户（组织钱包 + 成员角色 + 组织 Key）
- [x] P3-4 同步 TTS（`/v1/audio/speech`）
- [x] P4-1 双向实时语音（`/v1/realtime` WS）
- [x] P4-2 音乐适配器（suno 社区 API）
- [x] P4-3 每 Key 限流（RPM / TPM）
- [x] P5-1 通道自动故障切换（失败冷却 + 健康状态）
- [x] P6-1 任务管线故障切换（任务级 / 镜头级）

## 文档与社区

| 资源 | 链接 |
| --- | --- |
| 架构设计 | [docs/architecture.md](docs/architecture.md) |
| API 参考 | [docs/api.md](docs/api.md) |
| 贡献指南 | [CONTRIBUTING.md](CONTRIBUTING.md) |
| 问题反馈 | [GitHub Issues](https://github.com/1528960014/modelhub/issues) |

---

## 📜 许可证

[MIT](LICENSE)

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=1528960014/modelhub&type=Date)](https://star-history.com/#1528960014/modelhub&Date)

</div>

---

<div align="center">

### 💖 感谢使用 ModelHub

如果这个项目对你有帮助，欢迎点个 ⭐️ Star！

**[API 参考](docs/api.md)** • **[Issues](https://github.com/1528960014/modelhub/issues)** • **[贡献指南](CONTRIBUTING.md)**

</div>
