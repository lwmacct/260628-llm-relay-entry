package server

import (
	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/urfave/cli/v3"
)

var (
	defaults = config.DefaultConfig()
	usage    = cfgm.Schema(defaults).Command("server")
)

var Command = &cli.Command{
	Name:            "server",
	Usage:           "start HTTP server",
	Action:          action,
	Commands:        []*cli.Command{version.Command},
	HideHelpCommand: true,
	Flags:           commandFlags(),
}

func commandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: usage.MustUsage("debug"),
			Value: defaults.Server.Debug,
		},
		&cli.StringFlag{
			Name:  "http.listen",
			Usage: usage.MustUsage("http.listen"),
			Value: defaults.Server.HTTP.Listen,
		},
		&cli.StringFlag{
			Name:  "http.web-root",
			Usage: usage.MustUsage("http.web-root"),
			Value: defaults.Server.HTTP.WebRoot,
		},
		&cli.BoolFlag{
			Name:  "http.tls.enabled",
			Usage: usage.MustUsage("http.tls.enabled"),
			Value: defaults.Server.HTTP.TLS.Enabled,
		},
		&cli.StringFlag{
			Name:  "http.tls.cert-file",
			Usage: usage.MustUsage("http.tls.cert-file"),
			Value: defaults.Server.HTTP.TLS.CertFile,
		},
		&cli.StringFlag{
			Name:  "http.tls.key-file",
			Usage: usage.MustUsage("http.tls.key-file"),
			Value: defaults.Server.HTTP.TLS.KeyFile,
		},
		&cli.DurationFlag{
			Name:  "http.tls.poll-interval",
			Usage: usage.MustUsage("http.tls.poll-interval"),
			Value: defaults.Server.HTTP.TLS.PollInterval,
		},
		&cli.DurationFlag{
			Name:  "http.read-header-timeout",
			Usage: usage.MustUsage("http.read-header-timeout"),
			Value: defaults.Server.HTTP.ReadHeaderTimeout,
		},
		&cli.DurationFlag{
			Name:  "http.read-timeout",
			Usage: usage.MustUsage("http.read-timeout"),
			Value: defaults.Server.HTTP.ReadTimeout,
		},
		&cli.DurationFlag{
			Name:  "http.write-timeout",
			Usage: usage.MustUsage("http.write-timeout"),
			Value: defaults.Server.HTTP.WriteTimeout,
		},
		&cli.DurationFlag{
			Name:  "http.idle-timeout",
			Usage: usage.MustUsage("http.idle-timeout"),
			Value: defaults.Server.HTTP.IdleTimeout,
		},
		&cli.DurationFlag{
			Name:  "http.shutdown-timeout",
			Usage: usage.MustUsage("http.shutdown-timeout"),
			Value: defaults.Server.HTTP.ShutdownTimeout,
		},
		&cli.Int64Flag{
			Name:  "http.max-api-body-bytes",
			Usage: usage.MustUsage("http.max-api-body-bytes"),
			Value: defaults.Server.HTTP.MaxAPIBodyBytes,
		},
		&cli.BoolFlag{
			Name:  "http.enable-debug-requests",
			Usage: usage.MustUsage("http.enable-debug-requests"),
			Value: defaults.Server.HTTP.EnableDebugRequests,
		},
		&cli.StringFlag{
			Name:  "adapter.relay.base-url",
			Usage: usage.MustUsage("adapter.relay.base-url"),
			Value: defaults.Server.Adapter.Relay.BaseURL,
		},
		&cli.IntFlag{
			Name:  "adapter.relay.max-idle-conns",
			Usage: usage.MustUsage("adapter.relay.max-idle-conns"),
			Value: defaults.Server.Adapter.Relay.MaxIdleConns,
		},
		&cli.IntFlag{
			Name:  "adapter.relay.max-idle-conns-per-host",
			Usage: usage.MustUsage("adapter.relay.max-idle-conns-per-host"),
			Value: defaults.Server.Adapter.Relay.MaxIdleConnsPerHost,
		},
		&cli.IntFlag{
			Name:  "adapter.relay.max-conns-per-host",
			Usage: usage.MustUsage("adapter.relay.max-conns-per-host"),
			Value: defaults.Server.Adapter.Relay.MaxConnsPerHost,
		},
		&cli.DurationFlag{
			Name:  "adapter.relay.idle-conn-timeout",
			Usage: usage.MustUsage("adapter.relay.idle-conn-timeout"),
			Value: defaults.Server.Adapter.Relay.IdleConnTimeout,
		},
		&cli.BoolFlag{
			Name:  "adapter.relay.disable-keep-alives",
			Usage: usage.MustUsage("adapter.relay.disable-keep-alives"),
			Value: defaults.Server.Adapter.Relay.DisableKeepAlives,
		},
		&cli.DurationFlag{
			Name:  "adapter.relay.rate-limit-cooldown-ttl",
			Usage: usage.MustUsage("adapter.relay.rate-limit-cooldown-ttl"),
			Value: defaults.Server.Adapter.Relay.RateLimitCooldownTTL,
		},
		&cli.DurationFlag{
			Name:  "adapter.relay.rate-limit-retry-after",
			Usage: usage.MustUsage("adapter.relay.rate-limit-retry-after"),
			Value: defaults.Server.Adapter.Relay.RateLimitRetryAfter,
		},
		&cli.StringFlag{
			Name:  "adapter.runtime.api-base-url",
			Usage: usage.MustUsage("adapter.runtime.api-base-url"),
			Value: defaults.Server.Adapter.Runtime.APIBaseURL,
		},
		&cli.StringFlag{
			Name:  "adapter.runtime.auth-token",
			Usage: usage.MustUsage("adapter.runtime.auth-token"),
			Value: defaults.Server.Adapter.Runtime.AuthToken,
		},
		&cli.StringFlag{
			Name:  "adapter.runtime.plan-id",
			Usage: usage.MustUsage("adapter.runtime.plan-id"),
			Value: defaults.Server.Adapter.Runtime.PlanID,
		},
		&cli.DurationFlag{
			Name:  "adapter.runtime.resolve-timeout",
			Usage: usage.MustUsage("adapter.runtime.resolve-timeout"),
			Value: defaults.Server.Adapter.Runtime.ResolveTimeout,
		},
		&cli.DurationFlag{
			Name:  "adapter.runtime.report-timeout",
			Usage: usage.MustUsage("adapter.runtime.report-timeout"),
			Value: defaults.Server.Adapter.Runtime.ReportTimeout,
		},
		&cli.BoolFlag{
			Name:  "adapter.runtime.allow-partial-failover",
			Usage: usage.MustUsage("adapter.runtime.allow-partial-failover"),
			Value: defaults.Server.Adapter.Runtime.AllowPartialFailover,
		},
		&cli.BoolFlag{
			Name:  "token-auth.redis-bloom.enabled",
			Usage: usage.MustUsage("token-auth.redis-bloom.enabled"),
			Value: defaults.Server.TokenAuth.RedisBloom.Enabled,
		},
		&cli.StringFlag{
			Name:  "token-auth.redis-bloom.url",
			Usage: usage.MustUsage("token-auth.redis-bloom.url"),
			Value: defaults.Server.TokenAuth.RedisBloom.URL,
		},
		&cli.StringFlag{
			Name:  "token-auth.redis-bloom.password",
			Usage: usage.MustUsage("token-auth.redis-bloom.password"),
			Value: defaults.Server.TokenAuth.RedisBloom.Password,
		},
		&cli.StringFlag{
			Name:  "token-auth.redis-bloom.key-prefix",
			Usage: usage.MustUsage("token-auth.redis-bloom.key-prefix"),
			Value: defaults.Server.TokenAuth.RedisBloom.KeyPrefix,
		},
	}
}
