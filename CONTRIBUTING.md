# 贡献指南

感谢关注 ModelHub！

## 开发环境

- Go 1.24+（服务端）
- Node 20+ / pnpm（前端）
- Docker（PostgreSQL / Redis / MinIO）

```bash
cd deploy && docker compose up -d
cp .env.example .env
cd server && go mod tidy && go run ./cmd/modelhub
```

前端开发：

```bash
cd web && pnpm install && pnpm dev
```

Vite 已配置 `/v1` 与 `/admin` 代理到 `http://127.0.0.1:8080`。

## 代码约定

- 服务端：Go 官方格式（gofmt/goimports），错误用 `fmt.Errorf("%w")` 包装，数据库访问集中在 `internal/store`。
- 前端：TypeScript 严格模式；组件 PascalCase；文案一律走 i18n（`src/i18n/locales/*.json`），不要硬编码中文或英文。
- 新增上游 provider：在 `internal/gateway` 增加 adapter，并补单元测试。
- 提交信息：`feat/fix/docs/refactor/test/chore(scope): 简述`。

## 安全红线

- 任何真实 API key 不得进入仓库、日志或测试数据。
- 新增对外接口必须走 API key 或 MASTER_KEY 鉴权，并有限流考量。