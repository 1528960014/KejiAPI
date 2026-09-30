<div align="center">

# ModelHub

**模型、应用与智能体一体的自托管 AI 网关平台**

<p align="center">
  <a href="https://github.com/1528960014/modelhub/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/1528960014/modelhub?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://go.dev/">
    <img src="https://img.shields.io/badge/go-1.25-blue" alt="go">
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
  <a href="#特性">特性</a> •
  <a href="#接口一览">接口一览</a> •
  <a href="#技术栈">技术栈</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#生产部署">生产部署</a> •
  <a href="#开发与目录结构">开发</a> •
  <a href="#路线图">路线图</a>
</p>

</div>

---

## 📝 项目简介

ModelHub 是一个**自托管的 AI 网关**,适用于应用程序、代理和团队。它连接上游模型服务,向客户端提供**一致的 OpenAI 兼容 API**,并集中管理路由、访问、使用情况和成本。

您可以使用它在团队内共享授权模型访问权限,无需重新配置每个客户端即可切换服务提供商,或者通过 Web 控制台运行私有多模型服务。上游服务包括 OpenAI、Azure OpenAI、DeepSeek、通义千问(Qwen)、SiliconFlow、Moonshot 等 OpenAI 兼容服务——任何兼容端点都可以通过通道 Base URL 接入。

> [!IMPORTANT]
> - 本项目仅用于合法授权的 AI API 网关、组织级身份验证、多模型管理、使用情况分析、成本核算和私有部署场景。
> - 用户必须合法获取上游 API 密钥、账号、模型服务和接口权限,并且必须遵守上游服务条款和适用的法律法规。
> - 用户应确保其使用符合上游服务条款和适用的法律法规。
> - 当向公众提供生成式人工智能服务时,用户应遵守适用的监管要求,并履行其所在司法管辖区要求的所有备案、许可、内容安全、实名验证、日志保留、税务和上游授权义务。

> [!WARNING]
> 将本项目作为公开 API 转售服务运营前,请先完成所在辖区要求的全部备案、支付、内容安全与上游授权义务。请勿在 `.env` 或代码库中提交真实密钥。

---

<a id="特性"></a>

## 特性

- **多账户管理** - 同一模型可配置多个上游账户(通道),每个账户独立 API Key + Base URL,按优先级路由(多账户类型如 OAuth 见路线图 P7-1)
- **API 密钥分发** - 为用户生成和管理 API Key(sk- 前缀,服务端仅存哈希);用户控制台自助创建,代理商可创建分销子 Key(独立白名单/额度/有效期)
- **精准计费** - token 级用量跟踪与成本计算;账本式余额(预估 → 冻结 → 结算);图像/视频/音乐/语音按件计价;全部流水可审计
- **智能调度** - 通道优先级路由 + 自动故障切换(失败 5 分钟冷却),通道健康状态实时可见(会话粘性见路线图 P7-2)
- **速率限制与并发控制** - 可配置的每 Key 请求速率(RPM)与 token 速率(TPM);每 Key / 每上游通道(账户)在途并发上限,超限返回 429 `rate_limited` / `concurrency_limited`
- **内置支付系统** - 用户自助充值(人民币 → USD 余额),无需单独支付服务:开源易支付(聚合)/ 支付宝官方(当面付)/ 微信官方(v3 扫码),回调验签、下单锁价、幂等入账,渠道凭据后台热更新(Stripe 见路线图 P7-4)
- **管理后台** - 用于监控和管理的 Web 界面:仪表盘(统计卡 + 近 7 天成本图表 + 通道健康 + 最近请求),模型/通道/用户/Key/代理商/组织/智能体/支付/用量/充值订单全量管理,外部系统 iframe 嵌入扩展,客户端搜索与弹窗编辑
- **路由层** - 模型 → 多通道(多账户)的优先级路由与故障切换,同步接口与任务管线共享冷却状态(复合组见路线图 P7-5)
- **异步任务管线** - `/v1/media/generate` 图像/视频/音乐(suno 社区 API)/TTS 生成任务,DB 队列 + worker、按件冻结/结算、状态轮询、任务级故障切换
- **漫剧工坊** - 剧本 → LLM 分镜 → 逐镜画面+配音 → ffmpeg 合成带字幕 MP4 成片,all-or-nothing 计费,素材包 JSON 导出
- **智能体** - 预定义 agent 模板(system prompt + 绑定模型 + OpenAI function calling 工具注入),`model: agent_id` 直接调用、按真实模型计费
- **实时语音** - 同步 TTS(`/v1/audio/speech`,聊天朗读)+ 双向实时语音 WebSocket 透传(`/v1/realtime`,麦克风↔麦克风)
- **多租户组织** - 组织独立钱包 + 成员角色(owner/admin/member),组织 Key 按列表价计费,组织用量与流水审计
- **Web 控制台** - 多模型对比聊天(工具调用/智能体/朗读/实时语音)、生成工作台、API 控制台、自助充值;6 语言(中/英/日/韩/俄/西)、深浅主题

<a id="接口一览"></a>

## 接口一览

| 接口 | 常用端点 |
| --- | --- |
| OpenAI 兼容 | `POST /v1/chat/completions`(SSE)· `GET /v1/models` · `GET /v1/agents` |
| 实时语音 | `POST /v1/audio/speech` · `POST /v1/realtime`(WebSocket 升级) |
| 异步任务 | `POST /v1/media/generate` · `GET /v1/media/status/:task_id` |
| 漫剧 | `POST /v1/drama/generate` · `GET /v1/drama/status/:drama_id` · `GET /media/dramas/:uuid`(MP4) |
| 用户自助 | `POST /api/auth/{register,login,refresh,logout}` · `GET /api/me` · `/api/me/keys` · `/api/me/subkeys` · `/api/me/orgs/**` · `/api/me/recharges` |
| 管理 API(MASTER_KEY) | `/admin/models` · `/admin/channels` · `/admin/api-keys` · `/admin/users`(含 `/:id/credit`、`/:id/ledger`)· `/admin/agents` · `/admin/assistants` · `/admin/organizations` · `/admin/pay-config` · `/admin/usage`(含 `/summary`)· `/admin/tasks` · `/admin/dramas` · `/admin/recharges` |
| 支付回调 | `GET/POST /pay/notify/{yipay,alipay,wechat}` |

<a id="技术栈"></a>

## 技术栈

| 组件 | 技术 |
| --- | --- |
| 后端 | Go 1.25、Gin、gorilla/websocket、JWT |
| 前端 | Vue 3、Vite、Element Plus、vue-i18n(6 语言) |
| 数据库 | PostgreSQL 15+(pgx v5,账本/用量/任务全持久化) |
| 缓存/依赖 | Redis 7+(docker compose 内置) |
| 对象存储 | MinIO(媒体任务产物,docker compose 内置) |
| 媒体合成 | ffmpeg + ffprobe(漫剧 MP4 成片,镜像内置 Noto CJK 字体) |
| 容器化 | Docker / Docker Compose 一键部署 |

---

<a id="快速开始"></a>

## 快速开始

前置:Go 1.25+、Docker、pnpm(仅前端开发需要)。漫剧 MP4 成片需要主机安装 `ffmpeg`/`ffprobe`(`apt install ffmpeg`,中文字幕建议另装 CJK 字体)。

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

打开 `http://127.0.0.1:8080`(Web 控制台)或前端开发服务器,注册账号即可使用;管理后台粘贴 `MASTER_KEY` 解锁。

<a id="生产部署"></a>

## 生产部署

`deploy/` 提供完整编排(app + web + postgres + redis + minio),一条命令起站:

```bash
git clone https://github.com/1528960014/modelhub /www/modelhub && cd /www/modelhub
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env：MASTER_KEY（openssl rand -hex 32）、PAY_PUBLIC_URL 等
docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build
curl http://127.0.0.1/healthz
```

| 项目 | 说明 |
| --- | --- |
| 入口 | `web` 容器默认占 80 端口(SPA + `/v1`、`/admin` 反代到 `app`);`app` 直连 8080 可留作调试 |
| 端口冲突 | 宿主机 80 被占用(如宝塔 nginx):`./deploy/deploy.sh host80`(web 改映射 8000),再在 Web 服务器加反代 → `http://127.0.0.1:8000` |
| 一键脚本 | `deploy/deploy.sh`:校验 Docker、生成 `.env` 提示、构建启动、健康检查 |
| 数据 | 命名卷(`pgdata` / `mediadata` / `miniodata`);PostgreSQL / Redis / MinIO 只绑 `127.0.0.1` |
| 漫剧合成 | 镜像内置 ffmpeg + Noto CJK 字体 |
| TLS | 443 用面板或 certbot 再套一层 |

### 主要环境变量

| 变量 | 用途 |
| --- | --- |
| `MASTER_KEY` | 管理 API 与 Web 管理登录密钥,≥16 位,建议 `openssl rand -hex 32` |
| `PAY_PUBLIC_URL` | 公网地址(支付回调 / 回跳 URL 基于它拼接) |
| `MODELHUB_RPM` / `MODELHUB_TPM` | 每 Key 限流,0 = 关闭 |
| `MODELHUB_CONCURRENCY_PER_KEY` / `MODELHUB_CONCURRENCY_PER_CHANNEL` | 并发控制:每 Key / 每上游通道(账户)在途并发上限,0 = 关闭 |
| `PAY_CNY_PER_USD` | 充值汇率(0 = 关闭在线充值) |
| `PAY_YIPAY_*` | 易支付(聚合):`MAPI_URL` / `PID` / `KEY` |
| `PAY_ALIPAY_*` | 支付宝官方:`APP_ID` / `PRIVATE_KEY` / `PUBLIC_KEY` |
| `PAY_WECHAT_*` | 微信官方:`MCH_ID` / `APP_ID` / `APIV3_KEY` / `MERCHANT_SERIAL` / `PRIVATE_KEY` / `PLATFORM_KEY` |

支付渠道凭据也可在管理后台「支付配置」页热更新(env 仅作首次启动默认值),详见 [docs/api.md](docs/api.md)。

<a id="开发与目录结构"></a>

## 开发与目录结构

后端为 Go 单二进制(内嵌前端产物),前端为 Vue 3 + Vite + Element Plus + vue-i18n:

```bash
# 前端开发（代理 /api、/v1、/admin 到 8080）
cd web && pnpm install && pnpm dev

# 后端构建（前端先 build，server 内嵌 web/dist）
cd web && pnpm build
cd ../server && go build ./cmd/modelhub
```

| 目录 | 职责 |
| --- | --- |
| `server/internal/api` | HTTP 路由与处理器(v1 / admin / api / pay,基于 Gin) |
| `server/internal/gateway` | 上游适配(OpenAI 兼容透传)、流式转发、通道优先级故障切换 |
| `server/internal/billing` | 账本(冻结/结算)、token 级计价、RPM/TPM 限流 |
| `server/internal/store` | PostgreSQL 持久层(pgx;模型/通道/用户/Key/账本/用量/任务/组织/支付) |
| `server/internal/auth` | 用户 / 会话(JWT)/ API Key(仅存哈希) |
| `web/` | Web 控制台与管理员后台(模型浏览器 + 统一聊天/生成,6 语言 i18n) |
| `deploy/` | 生产编排:Dockerfile(server/web)+ docker-compose + nginx 模板 + 一键脚本 |
| `docs/` | [架构设计](docs/architecture.md) 与 [API 参考](docs/api.md) |

## 路线图

- [x] M1 网关核心(OpenAI 兼容聊天 / 流式 / API key)
- [x] M2 计费账本(余额冻结与结算、用量统计、管理端手动充值)
- [x] M3 Web 控制台(多模型聊天、API 控制台、i18n、主题)
- [x] M4 任务管线(图像 → 视频 / 音乐 / TTS,适配器全部落地)
- [x] P2-1 漫剧管线(分镜 + 素材包 + ffmpeg MP4 成片)
- [x] P2-2 智能体(agent 模板 + 工具调用)
- [x] P2-3 代理分销(批发价结算 + 分销子 Key)
- [x] P2-4 在线支付(易支付 / 支付宝官方 / 微信官方)
- [x] P2-5 更多语言(ja/ko/ru/es,共 6 语言)
- [x] P3-1 智能体工具调用(OpenAI function calling)
- [x] P3-2 支付渠道后台热配置
- [x] P3-3 多租户(组织钱包 + 成员角色 + 组织 Key)
- [x] P3-4 同步 TTS(`/v1/audio/speech`)
- [x] P4-1 双向实时语音(`/v1/realtime` WS)
- [x] P4-2 音乐适配器(suno 社区 API)
- [x] P4-3 每 Key 限流(RPM / TPM)
- [x] P5-1 通道自动故障切换(失败冷却 + 健康状态)
- [x] P6-1 任务管线故障切换(任务级 / 镜头级)
- [ ] P7-1 多账户类型(支持 OAuth 上游账户,当前为 API Key)
- [ ] P7-2 会话粘性智能调度(同会话稳定路由到同一账户)
- [x] P7-3 并发控制(每 Key / 每上游账户在途并发限制)
- [ ] P7-4 Stripe 支付渠道
- [ ] P7-5 复合组(将请求模型解析为多账户组内具体账户的路由层)
- [x] P7-6 外部系统集成(iframe 嵌入工单等外部系统扩展管理面板)

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

如果这个项目对你有帮助,欢迎点个 ⭐️ Star!

**[API 参考](docs/api.md)** • **[Issues](https://github.com/1528960014/modelhub/issues)** • **[贡献指南](CONTRIBUTING.md)**

</div>
