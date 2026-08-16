# LLM Relay Entry

LLM Relay Entry 是公开 API 数据面。它只处理 `POST /v1/responses`，不提供注册、Token 管理或前端页面，也不调用 Console HTTP API。

跨组件的完整 Token 与信任边界见
[LLM Relay 系统拓扑](https://github.com/lwmacct/260628-llm-relay-console/blob/main/docs/system-topology.md)。

## 请求链

```text
User llmr_ Token
  -> Entry 查询 Console PostgreSQL
  -> Entry 注入固定 dp.22.remote Token、Credential UUID 和派生亲和键
  -> directive-proxy
  -> Vendor 共享 listener 的 Relay 路由 /api/relay/resolver
  -> Vendor API
```

Entry 对每个请求执行一次按完整 Token 索引的数据库查询，并同时校验 API Key、用户、Binding、Key 分组、过期时间和 Key 限额。数据库只返回 Vendor Route Credential UUID，不返回 Vendor URL、连接池或上游密钥。

用户提供的 `Authorization`、Cookie、代理披露头、`X-Dp-*`、`X-Relay-Credential-ID` 和 `X-Resolver-Affinity-Key` 会在出站前删除。Entry 随后写入服务端固定 Remote Token、数据库解析出的 Credential UUID，以及由 Key ID 和可选 `Session-Id` 派生的亲和键。

这里的 Credential UUID 是 Vendor `RouteCredential.id`。Vendor 创建或轮换凭据时返回的
`remoteSpec.uuid` 当前与该 ID 相同；Relay Binding 应保存这个 UUID，不保存完整 RemoteSpec 或 `dpr_*`。
Entry 不验证 `dpr_*`，Vendor Relay resolver 也不要求它。

## 固定 Remote Token

固定 Token 的 RemoteSpec 只包含 Vendor Relay resolver URL 和 S2S Bearer，不包含 Route Credential UUID。
HTTP 调用实际由 directive-proxy 发起；S2S Bearer 在 Vendor 的 Relay 路由上验证。使用下列命令生成：

```bash
export DIRECTIVE_TOKEN_SECRET='same-secret-as-directive-proxy'
export RELAY_ENTRY_S2S_TOKEN='same-secret-as-vendor'
app remote-token --resolver-url 'http://127.0.0.1:23188/api/relay/resolver'
```

将输出写入 Entry 的 `DIRECTIVE_REMOTE_TOKEN`。不要把该值交给 API 用户或写入 Console 数据库。

`dp.22.remote` 的 JSON 使用 Base64URL 编码，可以直接解码；HMAC 只保证完整性，不提供加密。
因为该固定 Token 内含 S2S Bearer，所以它本身必须按明文密钥保护，禁止进入普通日志、聊天或前端配置。

## 配置

- `DIRECTIVE_REMOTE_TOKEN`：上一步生成的固定内部 `dp.22.remote` Token。
- `DIRECTIVE_PROXY_BASE_URL`：directive-proxy 的内部 HTTP 地址。
- `PGHOST`、`PGPORT`、`PGUSER`、`PGDATABASE`、`PGPASSWORD`：Console PostgreSQL。

Vendor 的 `/api/relay/resolver` 与公共 `/api/resolver` 共用 `server.http.listen`。默认同机部署可用
`http://127.0.0.1:23188` 省去本机 TLS；跨主机或经公网入口时改用可达的 HTTPS URL。无论传输方式如何，
Vendor 都会验证 S2S Bearer 和 Credential UUID，Entry 不验证 `dpr_*`。

Entry 直接以用户提交的完整 Token 查询 `api_keys.token`。PostgreSQL 账号应只拥有 `api_keys`、`users`、`relay_bindings`、`api_key_groups` 的 `SELECT` 权限；数据库备份和查询日志必须按敏感凭据保护。Entry 不执行建表或迁移。

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
