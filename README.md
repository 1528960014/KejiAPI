<div align="center">

# Kejike API

**模型、应用与智能体一体的自托管 AI 网关平台**

<p align="center">
  <a href="https://github.com/1528960014/KejiAPI/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/1528960014/KejiAPI?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://go.dev/">
    <img src="https://img.shields.io/badge/go-1.25-blue" alt="go">
  </a><!--
  --><a href="https://react.dev/">
    <img src="https://img.shields.io/badge/frontend-React%2019-brightgreen" alt="react">
  </a>
  <a href="https://docs.docker.com/compose/">
    <img src="https://img.shields.io/badge/docker-compose-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#项目简介">项目简介</a> •
  <a href="#特性">特性</a> •
  <a href="#快速开始">快速开始</a> •
  <a href="#生产部署">生产部署</a> •
  <a href="#主要环境变量">环境变量</a> •
  <a href="#目录结构">目录结构</a>
</p>

</div>

---

## 📝 项目简介

一个自托管的 AI 网关，适用于应用程序、代理和团队。它可以连接上游模型服务，向客户端提供**一致的 API**，并集中管理路由、访问、使用情况和成本。

您可以使用它在团队内共享授权模型访问权限，无需重新配置每个客户端即可切换服务提供商，或者通过 Web 控制台运行私有多模型服务。上游服务包括 OpenAI、Anthropic、Google Gemini、Azure OpenAI、AWS Bedrock、Vertex AI、DeepSeek、Qwen 以及其他兼容服务。

> [!IMPORTANT]
> 本项目仅用于合法授权的 AI API 网关、组织级身份验证、多模型管理、使用情况分析、成本核算和私有部署场景。
> 用户必须合法获取上游 API 密钥、账号、模型服务和接口权限，并且必须遵守上游服务条款和适用的法律法规。
> 用户应确保其使用符合上游服务条款和适用的法律法规。
> 当向公众提供生成式人工智能服务时，用户应遵守适用的监管要求，并履行其所在司法管辖区要求的所有备案、许可、内容安全、实名验证、日志保留、税务和上游授权义务。

> [!WARNING]
> 将本项目作为公开 API 转售服务运营前，请先完成所在辖区要求的全部备案、支付、内容安全与上游授权义务。请勿在 `.env` 或代码库中提交真实密钥。

---

## 特性

- **多账户管理** - 支持多种上游账户类型（OAuth、API 密钥），同一模型可挂多个上游账户/通道，按优先级与权重智能路由；浏览器授权（auth-url）一键接入订阅账号
- **账号池管理** - 聚合管理全部订阅账户（Gemini / ChatGPT(Codex) / Claude / Antigravity / 网关中转账户）：凭据状态、Token 有效期、一键刷新、启用停用、池统计
- **池级智能调度** - 会话粘性 + 负载感知：同一 API 密钥稳定路由到同一池账号；实时错误率（EWMA）超阈值自动逃逸到健康账号；并发槽、账号负载系数（load factor）、Top-K 健康加权抽选
- **防封引擎** - 封禁检测自动隔离、上游配额/限流/过载/凭据错误**信号分层冷却**（429 秒级、529 分钟级、401/403 分钟级）并定时恢复、每账户滑动窗口请求限速，防止账号被过度使用封禁
- **账号健康检查** - 每账号可设定时健康检查（间隔 + 测试模型）：失败自动禁用、成功自动恢复并清除冷却/隔离标记，检查结果留痕可查
- **订阅到期管理** - 账号可设到期时间，到期自动停用；导入时批量设置并发上限与到期天数
- **API 密钥分发** - 为用户生成和管理 API 密钥；用户控制台自助创建，支持分组额度与有效期
- **精准计费** - 代币级使用情况跟踪和成本计算；模型倍率/补差倍率可配，全部用量日志可审计
- **智能调度** - 智能选择账户并保持会话粘性；通道优先级、权重、自动故障切换与冷却，线路健康状态实时可见
- **并发控制** - 每个用户和每个账户的并发限制
- **速率限制** - 可配置的请求和令牌速率限制
- **内置支付系统** - 支持易支付（聚合支付宝/微信）、Stripe 等用户自助充值方式，无需单独的支付服务；兑换码/优惠码、订阅套餐、支付回调验签与幂等入账
- **管理后台** - 用于监控和管理的 Web 界面：仪表盘、渠道（通道）管理、模型管理、用户管理、令牌、额度、兑换、订阅、日志、系统设置
- **复合组** - 管理路由层，用于将请求的模型解析为多提供商组的具体提供商
- **外部系统集成** - 通过 iframe 嵌入外部系统（例如工单系统）以扩展管理控制面板
- **多模态** - 聊天补全、视觉理解、图像/音频等任务型接口统一接入；OpenAI 兼容 API，客户端零改造
- **Web 控制台** - 用户侧：自助充值、API 密钥、用量日志、邀请返利、订阅管理；6+ 语言与深浅主题

## 账号池管理（3.0 重点）

账号池把订阅类账号（OAuth 账户、中转网关账户）当作可调度资源池管理，覆盖「接入 → 调度 → 防封 → 恢复」全生命周期：

| 环节 | 能力 |
| --- | --- |
| 接入 | 粘贴导入（Claude/ChatGPT/Google 凭据，支持 JSON 数组 / 行 / 拼接）、浏览器授权流、批量并发上限与到期天数 |
| 调度 | 会话粘性（按 API 密钥）→ 错误率逃逸（EWMA）→ 并发槽 → Top-K 健康加权抽选，账号负载系数可调 |
| 防封 | 封禁隔离不自动恢复；429 秒级冷却；529 过载隔离；401/403 凭据分钟级冷却；每账号滑动窗口限速 |
| 恢复 | 冷却到期自动恢复；定时健康检查成功自动恢复并清除标记；失败自动禁用 |
| 运维 | 账号池页实时负载（活跃/错误率）、冷却分类、最近健康结果；全部参数可在「账号池 - 管理设置」调整 |

在后台「账号池」页即可完成上述全部操作；渠道管理页负责账号的凭据、模型、分组等基础配置。

## 快速开始

前置：Go 1.25+、Docker（二选一即可，推荐 Docker）。

```bash
# 方式一：Docker Compose 一键部署（含 PostgreSQL + Redis）
git clone https://github.com/1528960014/KejiAPI /www/kejiapi && cd /www/kejiapi
docker compose up -d --build
# 访问 http://localhost:3000 ，注册第一个账号即成为管理员
```

```bash
# 方式二：本地源码运行
cd web && pnpm install && pnpm build     # 前端产物嵌入后端
go build -o kejiapi .
./kejiapi                                 # 默认 :3000
```

首次访问注册管理员账号后：
1. 在「渠道」页添加上游账户（名称 / 类型 / 密钥 / 模型列表）
2. 在「模型」页配置价格倍率与可用性
3. 在「令牌」页为用户签发 API Key
4. 客户端按 OpenAI 兼容方式调用 `https://<your-host>/v1/chat/completions`

## 生产部署

`docker-compose.yml` 提供完整编排（app + postgres + redis），一条命令起站：

```bash
docker compose up -d --build
```

| 项目 | 说明 |
| --- | --- |
| 入口 | 单容器同时提供 API 与 Web 界面，默认 3000 端口 |
| 反代 | Nginx/宝塔 反代 `80/443 → 127.0.0.1:3000` 即可（建议 TLS） |
| 数据库 | 默认 PostgreSQL 15（命名卷持久化）；可切 MySQL / SQLite |
| 会话 | Redis（可配密码）；单机小规模可省略 |
| TLS | 443 用面板或 certbot 再套一层 |
| 日志 | `--log-dir /app/logs` 输出到挂载卷；错误日志可开启入库 |

### 主要环境变量

| 变量 | 用途 |
| --- | --- |
| `PORT` | 监听端口，默认 3000 |
| `SQL_DSN` | 数据库连接串（PostgreSQL / MySQL / SQLite 均可） |
| `REDIS_CONN_STRING` | Redis 连接串（可选，建议生产配置） |
| `SESSION_SECRET` | 多机部署必填：会话签名密钥 |
| `FRONTEND_BASE_URL` | 前端公网地址（邮件/回调链接拼接） |
| `EPAY_SERVER` / `EPAY_PID` / `EPAY_KEY` | 易支付（聚合支付宝/微信）充值 |
| `STRIPE_SECRET_KEY` | Stripe 充值 |
| `NOTIFY_ENABLED` / `SMTP_*` | 邮件通知（充值成功/失败等） |

完整变量说明见 [.env.example](.env.example)。

### 升级到 3.0

```bash
cd /www/kejiapi
git pull                      # 或下载新版本 Release 覆盖
docker compose build --no-cache app    # 版本升级建议不使用缓存构建
docker compose up -d
```

升级后无需手工迁移：账号池设置、并发上限、健康检查等新能力在首次保存「账号池 - 管理设置」后自动生效；数据库结构随启动自动迁移。

### 部署后检查清单

1. `docker compose ps` 三个容器均为 running（healthy）
2. 访问 Web 控制台，用初始化管理员登录（首次注册的账号即管理员）
3. 「渠道」页至少添加一个渠道并测试通过
4. 「账号池」页可见订阅账号、负载与设置项
5. 「令牌」页签发一把 API Key，用 OpenAI 兼容方式调用 `/v1/chat/completions` 验证

## 目录结构

后端为 Go 单二进制（内嵌前端产物），前端为 React 19 + rsbuild + shadcn/ui：

```bash
# 前端开发（代理 API 到 3000）
cd web && pnpm install && pnpm dev

# 后端构建（前端先 build，main.go 内嵌 web/dist）
cd web && pnpm build
cd .. && go build -o kejiapi .
```

| 目录 | 职责 |
| --- | --- |
| `controller/` | HTTP 路由处理器（relay / 管理 / 用户 / 支付 / 兑换 / 订阅） |
| `relay/` | 上游适配与转发（OpenAI / Claude / Gemini / Bedrock 等，流式与任务） |
| `relay/channel/` | 各上游渠道适配器（含 OAuth 渠道） |
| `model/` | 数据层（GORM：用户/渠道/令牌/额度/日志/订单/订阅） |
| `service/` | 业务逻辑（计费、通知、任务轮询、额度） |
| `setting/` | 运行时配置（倍率、限流、支付、功能开关） |
| `web/` | Web 控制台与管理员后台（React，多语言 i18n） |
| `docker-compose.yml` / `Dockerfile` | 生产编排与镜像构建 |
| `docs/` | OpenAPI 规范与部署文档 |

## 文档

| 资源 | 链接 |
| --- | --- |
| 部署文档 | [docs/](docs/) |
| 问题反馈 | [GitHub Issues](https://github.com/1528960014/KejiAPI/issues) |

---

## 📜 许可证

[MIT](LICENSE)

## 🌟 Star History

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=1528960014/KejiAPI&type=Date)](https://star-history.com/#1528960014/KejiAPI&Date)

</div>

---

<div align="center">

### 💖 感谢使用 Kejike API

如果这个项目对你有帮助，欢迎点个 ⭐️ Star!

</div>
