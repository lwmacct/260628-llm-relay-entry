package server

import (
	"errors"
	"strings"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
)

func validateConfig(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	if strings.TrimSpace(cfg.Server.Adapter.Relay.BaseURL) == "" {
		return errors.New("adapter.relay.base-url is required")
	}
	if strings.TrimSpace(cfg.Server.Adapter.Runtime.APIBaseURL) == "" {
		return errors.New("adapter.runtime.api-base-url is required")
	}
	if strings.TrimSpace(cfg.Server.Adapter.Runtime.PlanID) == "" {
		return errors.New("adapter.runtime.plan-id is required")
	}
	if cfg.Server.HTTP.ShutdownTimeout <= 0 {
		return errors.New("http.shutdown-timeout must be greater than zero")
	}
	return validateHTTPTLS(cfg.Server.HTTP.TLS)
}

func validateHTTPTLS(tlsConfig config.ServerHTTPTLS) error {
	if !tlsConfig.Enabled {
		return nil
	}
	if (tlsConfig.CertFile == "") != (tlsConfig.KeyFile == "") {
		return errors.New("http tls.cert-file and tls.key-file must be configured together")
	}
	if tlsConfig.AutoReload && tlsConfig.ReloadInterval <= 0 {
		return errors.New("http tls.reload-interval must be greater than zero when http tls.auto-reload is true")
	}
	return nil
}
