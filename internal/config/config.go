package config

import "time"

type Config struct {
	Server Server `json:"server" desc:"服务端配置"`
}

type Server struct {
	Debug     bool       `json:"debug"      desc:"启用调试日志和诊断信息"`
	HTTP      ServerHTTP `json:"http"       desc:"HTTP 服务配置"`
	Adapter   Adapter    `json:"adapter"    desc:"Codex relay entry 适配配置"`
	TokenAuth TokenAuth  `json:"token-auth" desc:"入口 token 鉴权配置"`
}

type ServerHTTP struct {
	Listen              string        `json:"listen"                desc:"HTTP 服务监听地址"`
	WebRoot             string        `json:"web-root"              desc:"静态 Web 根目录，留空则不托管前端"`
	TLS                 ServerHTTPTLS `json:"tls"                   desc:"HTTPS TLS 配置"`
	ReadHeaderTimeout   time.Duration `json:"read-header-timeout"   desc:"HTTP 请求头读取超时"`
	ReadTimeout         time.Duration `json:"read-timeout"          desc:"HTTP 请求读取超时；0 表示不限制，适合流式入口"`
	WriteTimeout        time.Duration `json:"write-timeout"         desc:"HTTP 响应写入超时；0 表示不限制，适合长流式响应"`
	IdleTimeout         time.Duration `json:"idle-timeout"          desc:"HTTP 空闲连接超时时间"`
	ShutdownTimeout     time.Duration `json:"shutdown-timeout"      desc:"优雅关闭超时时间"`
	MaxAPIBodyBytes     int64         `json:"max-api-body-bytes"    desc:"普通 HTTP API 最大请求体字节数，0 表示不限制；Codex relay 入口不使用该限制"`
	EnableDebugRequests bool          `json:"enable-debug-requests" desc:"调试日志级别下记录请求元数据，不记录 body 和敏感头"`
}

type ServerHTTPTLS struct {
	Enabled        bool          `json:"enabled"         desc:"是否启用 HTTPS TLS"`
	CertFile       string        `json:"cert-file"       desc:"TLS 证书文件路径"`
	KeyFile        string        `json:"key-file"        desc:"TLS 私钥文件路径"`
	AutoReload     bool          `json:"auto-reload"     desc:"是否自动重载 TLS 证书文件"`
	ReloadInterval time.Duration `json:"reload-interval" desc:"TLS 证书文件自动重载检查间隔"`
}

type Adapter struct {
	Relay   AdapterRelay   `json:"relay"   desc:"下游 relay 配置"`
	Runtime AdapterRuntime `json:"runtime" desc:"context-chain Runtime 配置"`
}

type AdapterRelay struct {
	BaseURL              string        `json:"base-url"                desc:"下游 relay base URL"`
	MaxIdleConns         int           `json:"max-idle-conns"          desc:"下游 relay 全局空闲连接池容量；只影响连接复用，不限制活跃并发"`
	MaxIdleConnsPerHost  int           `json:"max-idle-conns-per-host" desc:"下游 relay 单 host 空闲连接池容量；只影响连接复用，不限制活跃并发"`
	MaxConnsPerHost      int           `json:"max-conns-per-host"      desc:"下游 relay 单 host 活跃连接上限；0 表示不限制"`
	IdleConnTimeout      time.Duration `json:"idle-conn-timeout"       desc:"下游 relay 空闲连接在连接池中的保留时间"`
	DisableKeepAlives    bool          `json:"disable-keep-alives"     desc:"是否禁用下游 relay keep-alive"`
	RateLimitCooldownTTL time.Duration `json:"rate-limit-cooldown-ttl" desc:"Relay 返回 429 后向 runtime 上报的资源冷却时间"`
	RateLimitRetryAfter  time.Duration `json:"rate-limit-retry-after"  desc:"Relay 返回 429 后返回客户端的 Retry-After"`
}

type AdapterRuntime struct {
	APIBaseURL           string        `json:"api-base-url"            desc:"context-chain Runtime HTTP API base URL"`
	AuthToken            string        `json:"auth-token"              desc:"Runtime HTTP API Bearer token，为空则不发送"`
	PlanID               string        `json:"plan-id"                 desc:"Runtime resolve 使用的 plan_id"`
	ResolveTimeout       time.Duration `json:"resolve-timeout"         desc:"Runtime resolve HTTP 超时"`
	ReportTimeout        time.Duration `json:"report-timeout"          desc:"Runtime report HTTP 超时"`
	AllowPartialFailover bool          `json:"allow-partial-failover"  desc:"是否允许 Runtime 部分失败降级返回"`
}

type TokenAuth struct {
	RedisBloom TokenAuthRedisBloom `json:"redis-bloom" desc:"基于 Redis Bloom Filter 的 token 鉴权配置"`
}

type TokenAuthRedisBloom struct {
	Enabled   bool   `json:"enabled"    desc:"是否启用 Redis Bloom Filter token 鉴权"`
	URL       string `json:"url"        desc:"Redis URL，支持 REDIS_URL 环境变量"`
	Password  string `json:"password"   desc:"Redis 密码，支持 REDISCLI_AUTH 环境变量"`
	KeyPrefix string `json:"key-prefix" desc:"Redis key 前缀"`
}

func DefaultConfig() Config {
	return Config{
		Server: Server{
			HTTP: ServerHTTP{
				Listen:  ":40172",
				WebRoot: "",
				TLS: ServerHTTPTLS{
					Enabled:        false,
					CertFile:       "${APP_DATA:-.local/data}/ssl/fullchain.pem",
					KeyFile:        "${APP_DATA:-.local/data}/ssl/privkey.pem",
					AutoReload:     true,
					ReloadInterval: 3 * time.Second,
				},
				ReadHeaderTimeout:   10 * time.Second,
				ReadTimeout:         0,
				WriteTimeout:        0,
				IdleTimeout:         time.Minute,
				ShutdownTimeout:     15 * time.Second,
				MaxAPIBodyBytes:     1 << 20,
				EnableDebugRequests: true,
			},
			Adapter: Adapter{
				Relay: AdapterRelay{
					BaseURL:              "${CODING_ADAPTER_RELAY_BASE_URL:-http://localhost:40174}",
					MaxIdleConns:         4096,
					MaxIdleConnsPerHost:  2048,
					MaxConnsPerHost:      0,
					IdleConnTimeout:      time.Minute,
					DisableKeepAlives:    false,
					RateLimitCooldownTTL: 5 * time.Minute,
					RateLimitRetryAfter:  2 * time.Second,
				},
				Runtime: AdapterRuntime{
					APIBaseURL:           "${CONTEXT_CHAINS_API_BASE_URL:-http://localhost:40173}",
					AuthToken:            "${CONTEXT_CHAINS_HTTP_TOKEN:-lwmacct}",
					PlanID:               "${CONTEXT_CHAINS_PLAN_ID:-default}",
					ResolveTimeout:       10 * time.Second,
					ReportTimeout:        2 * time.Second,
					AllowPartialFailover: false,
				},
			},
			TokenAuth: TokenAuth{
				//nolint:gosec // Defaults are environment variable expressions and Redis key names, not hardcoded credentials.
				RedisBloom: TokenAuthRedisBloom{
					Enabled:   true,
					URL:       "${REDIS_URL:-redis://localhost:6379/0}",
					Password:  "${REDISCLI_AUTH}",
					KeyPrefix: "token:white",
				},
			},
		},
	}
}
