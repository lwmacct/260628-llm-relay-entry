# LLM Relay Entry

LLM Relay Entry 是公开 API 数据面。它只处理 `POST /v1/responses`，不提供注册、Token 管理或前端页面，也不调用 Console HTTP API。

## 请求链

```text
User llmr_ Token
  -> Entry 查询 Console PostgreSQL
  -> Entry 注入固定 dp.22.remote Token、Credential UUID 和派生亲和键
  -> directive-proxy
  -> Vendor /api/internal/resolver
  -> Vendor API
```

Entry 对每个请求执行一次按 HMAC 摘要索引的数据库查询，并同时校验 API Key、用户、Binding、Key 分组、过期时间和 Key 限额。数据库只返回 Vendor Route Credential UUID，不返回 Vendor URL、连接池或上游密钥。

用户提供的 `Authorization`、Cookie、代理披露头、`X-Dp-*`、`X-Relay-Credential-ID` 和 `X-Resolver-Affinity-Key` 会在出站前删除。Entry 随后写入服务端固定 Remote Token、数据库解析出的 Credential UUID，以及由 Key ID 和可选 `Session-Id` 派生的亲和键。

## 固定 Remote Token

固定 Token 的 RemoteSpec 只包含 Vendor 内部 resolver URL 和 S2S Bearer，不包含 Route Credential UUID。使用下列命令生成：

```bash
export DIRECTIVE_TOKEN_SECRET='same-secret-as-directive-proxy'
export RELAY_ENTRY_S2S_TOKEN='same-secret-as-vendor'
app remote-token --resolver-url 'http://llm-relay-vendor:23188/api/internal/resolver'
```

将输出写入 Entry 的 `DIRECTIVE_REMOTE_TOKEN`。不要把该值交给 API 用户或写入 Console 数据库。

## 配置

- `RELAY_TOKEN_DIGEST_KEY`：Console 和 Entry 共享的 Token HMAC 密钥，生产环境必填。
- `DIRECTIVE_REMOTE_TOKEN`：上一步生成的固定内部 `dp.22.remote` Token。
- `DIRECTIVE_PROXY_BASE_URL`：directive-proxy 的内部 HTTP 地址。
- `PGHOST`、`PGPORT`、`PGUSER`、`PGDATABASE`、`PGPASSWORD`：Console PostgreSQL。

Entry 的 PostgreSQL 账号应只拥有 `api_keys`、`users`、`relay_bindings`、`api_key_groups` 的 `SELECT` 权限。Entry 不执行建表或迁移。

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
