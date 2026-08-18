# LLM Relay Entry

LLM Relay Entry 是公开 API 数据面。它只处理 `POST /v1/responses`，不提供注册、Token 管理或前端页面，也不调用 Console HTTP API。

跨组件的完整 Token 与信任边界见
[LLM Relay 系统拓扑](https://github.com/lwmacct/260628-llm-relay-console/blob/main/docs/system-topology.md)。

## 请求链

```text
User sk-rdp-v1- Token (43-character alphanumeric suffix)
  -> Entry 查询 Console PostgreSQL
  -> Entry 读取 Key Group HTTP RemoteSpec，运行时签发 dp.22.remote 和派生亲和键
  -> directive-proxy
  -> Vendor 统一 resolver /api/resolver（dpr_* 认证）
  -> Vendor API
```

Entry 对每个请求执行一次按完整 Token 索引的数据库查询，并同时校验 API Key、用户、Key 分组、过期时间和 Key 限额。数据库返回 Key Group 的完整 HTTP RemoteSpec；它包含 Vendor resolver URL 和 dpr_* header，不包含 Vendor Route UUID、连接池或上游 API credential。

用户提供的 `Authorization`、Cookie、代理披露头、`X-Dp-*` 和 `X-Resolver-Affinity-Key` 会在出站前删除。Entry 随后写入运行时签发的 Remote Token，以及由 Key ID 和可选 `Session-Id` 派生的亲和键。

Vendor 的 `RelayTarget.id` 只存在于 Vendor 自身的 Target 管理流程中；Vendor 可在内部切换或删除当前 Directive Route，而不要求 Entry 保存 Route UUID。Entry 不解析或修改 RemoteSpec，只负责读取、校验并签名。

## 运行时签名

Key Group 保存 HTTP RemoteSpec JSON，Entry 每个请求读取并用 `DIRECTIVE_HMAC_SECRET` 运行时签发
`dp.22.remote`。RemoteSpec 的 HTTP header 直接携带 Vendor 签发的 `Bearer dpr_*`。
`dp.22.remote` 的 JSON 使用 Base64URL 编码，HMAC 只保证完整性，不提供加密；签名 token 不落库，
也不返回给 API 用户。

## 配置

- `DIRECTIVE_HMAC_SECRET`：与 directive-proxy 共享的 RemoteSpec HMAC secret。
- `DIRECTIVE_PROXY_BASE_URL`：directive-proxy 的内部 HTTP 地址。
- `PGHOST`、`PGPORT`、`PGUSER`、`PGDATABASE`、`PGPASSWORD`：Console PostgreSQL。

Vendor 只接受 `dpr_*` resolver token。默认同机部署可用 loopback HTTP；跨主机或经公网入口时改用可达的 HTTPS URL。

Entry 直接以用户提交的完整 Token 查询 `api_keys.token`。PostgreSQL 账号应只拥有 `api_keys`、`users`、`api_key_groups` 的 `SELECT` 权限；数据库备份和查询日志必须按敏感凭据保护。Entry 不执行建表或迁移。

## 运维

- `GET /livez`：进程存活，不访问外部依赖。
- `GET /readyz`：检查接流状态和 PostgreSQL 连接。
- 收到关闭信号后先撤销 readiness，再由 `http.Server.Shutdown` 等待活动流式请求，最长等待 `server.http.shutdown-timeout`。
- 流式代理使用 `FlushInterval: -1`，不会缓冲 SSE chunk。

验证：

```bash
go test ./...
go test -count=1 ./internal/testutil/tddcheck
```
