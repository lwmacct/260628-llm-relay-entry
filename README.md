# LLM Relay Entry

本服务是一个兼容 Codex 的 relay 入口，位于 Codex 客户端、下游 relay 和 context-chain runtime 之间。

## 请求链路

- `POST /v1/responses` 只接受 Codex 客户端请求。
- 入口校验 `Authorization: Bearer <token>` 和 `Session-Id`，同时兼容内部别名 `Session_id`。
- 可选通过 Redis Bloom Filter 校验原始 token 是否允许进入。
- Runtime resolve 使用 `token + session_id + plan_id` 解析出不透明的 relay credential payload。
- relay credential payload 会被 base64url 编码，并写入转发请求的 `Authorization` header。
- 入站原始 token 会通过 `M-Runtime-Key` 继续转发给下游 relay。
- relay 请求路径会被重写为 `/responses`，同时保留 query 参数。
- relay `2xx` 响应原样透传，包括流式响应。
- relay 非 `2xx` 响应 body 不会暴露给客户端；服务只记录清洗后的日志，并返回安全 JSON。
- relay `429` 会被视为资源冷却信号：服务通知 runtime，并向客户端返回可重试的 `503`。

## 架构

- `internal/appcmd/server`：CLI 命令、运行时装配、HTTP server、TLS reload、优雅关闭。
- `internal/config`：基于 cfgm 的配置 schema 和生成的示例配置。
- `internal/handler`：HTTP 边界、Huma `/api/health`、原始 Codex relay 入口。
- `internal/service`：Codex 入口准入校验和转发请求准备。
- `internal/infra/runtime`：context-chain Runtime HTTP client。
- `internal/infra/relay`：反向代理、credential payload 编码、响应安全化。
- `internal/infra/tokenauth`：Redis Bloom token checker。
- `internal/testutil/tddcheck`：架构规则检查。

## 启动

```shell
go run . server
```
