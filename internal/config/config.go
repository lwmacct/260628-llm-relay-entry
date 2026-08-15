package config

import (
	"time"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
)

type Config struct {
	Server Server `json:"server" desc:"服务端配置"`
}

type Server struct {
	Debug    bool           `json:"debug"    desc:"启用调试日志和诊断信息"`
	HTTP     ServerHTTP     `json:"http"     desc:"HTTP 服务配置"`
	Database ServerDatabase `json:"database" desc:"Console PostgreSQL 数据库配置"`
	Relay    ServerRelay    `json:"relay"    desc:"Directive Proxy 数据面配置"`
	Token    ServerToken    `json:"token"    desc:"Relay API Token 摘要配置"`
}

type ServerHTTP struct {
	Listen              string           `json:"listen"                desc:"HTTP 服务监听地址"`
	TLS                 tlsreload.Config `json:"tls"                   desc:"HTTPS TLS 配置"`
	ReadHeaderTimeout   time.Duration    `json:"read-header-timeout"   desc:"HTTP 请求头读取超时"`
	ReadTimeout         time.Duration    `json:"read-timeout"          desc:"HTTP 请求读取超时；0 表示不限制，适合流式入口"`
	WriteTimeout        time.Duration    `json:"write-timeout"         desc:"HTTP 响应写入超时；0 表示不限制，适合长流式响应"`
	IdleTimeout         time.Duration    `json:"idle-timeout"          desc:"HTTP 空闲连接超时时间"`
	ShutdownTimeout     time.Duration    `json:"shutdown-timeout"      desc:"优雅关闭并等待流式请求完成的最长时间"`
	EnableDebugRequests bool             `json:"enable-debug-requests" desc:"调试级别记录请求元数据，不记录 body 和敏感头"`
}

type ServerDatabase struct {
	Host         string `json:"host"          desc:"PostgreSQL 主机"`
	Port         string `json:"port"          desc:"PostgreSQL 端口"`
	User         string `json:"user"          desc:"PostgreSQL 用户名"`
	Database     string `json:"database"      desc:"PostgreSQL 数据库名"`
	Password     string `json:"password"      desc:"PostgreSQL 密码"`
	MaxOpenConns int    `json:"max-open-conns" desc:"PostgreSQL 最大连接数"`
	MaxIdleConns int    `json:"max-idle-conns" desc:"PostgreSQL 最大空闲连接数"`
}

type ServerRelay struct {
	BaseURL             string        `json:"base-url"                desc:"Directive Proxy HTTP 基址"`
	DirectiveToken      string        `json:"directive-token"         desc:"固定 dp.22.remote Token，仅在 Entry 到 Directive Proxy 之间使用"`
	MaxIdleConns        int           `json:"max-idle-conns"          desc:"下游全局空闲连接池容量"`
	MaxIdleConnsPerHost int           `json:"max-idle-conns-per-host" desc:"下游单主机空闲连接池容量"`
	MaxConnsPerHost     int           `json:"max-conns-per-host"      desc:"下游单主机活跃连接上限；0 表示不限制"`
	IdleConnTimeout     time.Duration `json:"idle-conn-timeout"       desc:"下游空闲连接保留时间"`
	DisableKeepAlives   bool          `json:"disable-keep-alives"     desc:"是否禁用下游 keep-alive"`
}

type ServerToken struct {
	DigestKeyID string `json:"digest-key-id" desc:"Token 摘要密钥版本，不得包含下划线"`
	DigestKey   string `json:"digest-key"    desc:"Token HMAC-SHA256 摘要密钥，必须与 Console 一致"`
}

func DefaultConfig() Config {
	return Config{Server: Server{
		HTTP: ServerHTTP{
			Listen: ":23108",
			TLS: func() tlsreload.Config {
				cfg := tlsreload.DefaultConfig()
				cfg.Enabled = false
				cfg.DefaultCertificate = "default"
				cfg.Certificates = []tlsreload.CertificateSource{
					{
						ID:          "default",
						Certificate: "${APP_DATA:-.local/data}/ssl/fullchain.pem",
						PrivateKey:  "${APP_DATA:-.local/data}/ssl/privkey.pem",
					},
				}
				return cfg
			}(),
			ReadHeaderTimeout:   10 * time.Second,
			ReadTimeout:         0,
			WriteTimeout:        0,
			IdleTimeout:         time.Minute,
			ShutdownTimeout:     15 * time.Second,
			EnableDebugRequests: true,
		},
		Database: ServerDatabase{
			Host: "${PGHOST}", Port: "${PGPORT}", User: "${PGUSER}", Database: "${PGDATABASE}", Password: "${PGPASSWORD}",
			MaxOpenConns: 32, MaxIdleConns: 16,
		},
		Relay: ServerRelay{
			BaseURL:             "${DIRECTIVE_PROXY_BASE_URL:-http://localhost:23198}",
			DirectiveToken:      "${DIRECTIVE_REMOTE_TOKEN:?fixed dp.22.remote token is required}",
			MaxIdleConns:        4096,
			MaxIdleConnsPerHost: 2048,
			IdleConnTimeout:     time.Minute,
		},
		Token: ServerToken{
			DigestKeyID: "v1",
			DigestKey:   "${RELAY_TOKEN_DIGEST_KEY:?relay token digest key is required}",
		},
	}}
}

var Manager = cfgm.New(
	DefaultConfig(),
	cfgm.AppName("app"),
)
