# 架构设计

## 总体结构

```
浏览器 (Vue3 SPA)
   │ HTTPS
   ▼
Nginx/Traefik（静态资源 + 反代）
   └── /v1/*、/admin/*  →  Gateway (Go, 单二进制)
        ├── 认证（API key / MASTER_KEY）
        ├── 模型网关
        │    ├── Provider 适配器（OpenAI 兼容透传；Anthropic 等后续）
        │    ├── 路由：model → channel（priority + 故障切换）
        │    └── 计费钩子（估算→冻结→结算，M2）
        ├── 任务管线（M4：worker → 上游任务 → 产物落 MinIO）
        └── 管理 API（用户、key、模型、通道、站点配置）

PostgreSQL（用户/账本/模型/通道/任务）  Redis（限流/队列，M2+）  MinIO（媒体产物）
```

## 模块说明

### 模型网关（M1）
- **model**：对外模型 ID、provider、上游模型名、能力标签、计价（每 1K token）。
- **channel**：同一 model 的上游接入点（base_url + api_key + priority），可多通道故障切换。
- 请求流：API key 鉴权 → 查 model → 选 channel → 替换 body 中 model 为上游名 → 透传转发（SSE 直写回）→ 记录 usage。

### 计费（M2）
- 账本式：`users.balance`（微美元整数，1 USD = 10^6 µ）+ `ledger_entries`（credit/hold/release/debit 有符号流水），余额 = 流水汇总，同事务更新。
- 请求生命周期：预估费用（prompt ≈ 字节数/4，completion ≈ max_tokens，默认 1024 上限 16384）→ 行锁下检查余额并 hold → 完成后 release + 按实际 token debit（chat 按真实 usage；流式注入 `stream_options.include_usage` 取末 chunk usage，取不到则按预估计费）→ 失败全额 release。
- 实际超过冻结额时允许透支（余额可为负），账本始终一致。
- API key 绑定用户 + 模型白名单（allowed_models）+ 额度（quota 微美元，消费累计在 api_keys.spend）+ 过期时间；`sk-` 前缀，库内只存 SHA-256。
- 管理端：创建用户、手动充值（ledger credit）、账本流水查询、用量明细/汇总。

### 任务管线（M4）
- task 表（uuid、type、status、payload、result_urls、cost）；Redis 队列 + worker。
- 产物下载至 MinIO，对外返回签名 URL；统一轮询 `/v1/media/status/{id}`。

### Web 控制台（M3）
- 多模型对比聊天（同一 prompt 并排 N 个模型，SSE 流式渲染）
- 生成工作台（图/视频/音乐/TTS 表单 + 任务列表 + 结果预览）
- API 控制台（key 管理、用量与消费）
- i18n zh-CN/en-US；深浅主题

## 核心数据表

users、ledger_entries、models、channels、api_keys、tasks、usage_logs

## 技术选型

| 决策点 | 选择 | 理由 |
| --- | --- | --- |
| 网关语言 | Go (Gin) | 流式代理性能、单二进制部署 |
| 前端 | Vue3 + Vite + TS + Element Plus + vue-i18n | 生态与贡献者熟悉度 |
| 数据库 | PostgreSQL 16 | JSONB、事务、成熟 |
| 队列/缓存 | Redis 7 | v1 量级足够，避免引入 MQ |
| 媒体存储 | MinIO (S3 API) | docker 内可跑，生产可换 S3/R2 |
| 许可证 | MIT | 采用率优先 |