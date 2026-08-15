package server

import (
	"errors"
	"strings"

	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
)

func validateConfig(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("config is required")
	}
	if strings.TrimSpace(cfg.Server.Relay.BaseURL) == "" || !strings.HasPrefix(strings.TrimSpace(cfg.Server.Relay.DirectiveToken), "dp.22.remote.") {
		return errors.New("relay base URL and fixed dp.22.remote token are required")
	}
	if strings.TrimSpace(cfg.Server.Token.DigestKeyID) == "" || strings.Contains(cfg.Server.Token.DigestKeyID, "_") || strings.TrimSpace(cfg.Server.Token.DigestKey) == "" {
		return errors.New("valid token digest settings are required")
	}
	if cfg.Server.Database.MaxOpenConns <= 0 || cfg.Server.Database.MaxIdleConns < 0 || cfg.Server.HTTP.ShutdownTimeout <= 0 {
		return errors.New("database connection limits and HTTP shutdown timeout are invalid")
	}
	return validateHTTPTLS(cfg.Server.HTTP.TLS)
}

func validateHTTPTLS(tlsConfig tlsreload.Config) error {
	if !tlsConfig.Enabled {
		return nil
	}
	return tlsConfig.Validate()
}
