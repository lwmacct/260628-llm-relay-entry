package server

import (
	"context"
	"fmt"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/runtime"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/tokenauth"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

type runtimeState struct {
	runtimeClient *runtime.Client
	resolver      *runtimeCredentialResolver
	tokenChecker  tokenauth.Checker
	relayProxy    *relay.Proxy
	codexEntries  *service.CodexEntryService
	tls           *tlsRuntime
}

func newRuntime(ctx context.Context, cfg *config.Config) (*runtimeState, error) {
	runtimeClient, err := runtime.NewClient(runtime.Config{
		BaseURL:        cfg.Server.Adapter.Runtime.APIBaseURL,
		AuthToken:      cfg.Server.Adapter.Runtime.AuthToken,
		ResolveTimeout: cfg.Server.Adapter.Runtime.ResolveTimeout,
		ReportTimeout:  cfg.Server.Adapter.Runtime.ReportTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure runtime client: %w", err)
	}

	resolver, err := newRuntimeCredentialResolver(
		runtimeClient,
		cfg.Server.Adapter.Runtime.PlanID,
		cfg.Server.Adapter.Runtime.AllowPartialFailover,
	)
	if err != nil {
		return nil, fmt.Errorf("configure runtime resolver: %w", err)
	}

	tokenChecker, err := tokenauth.NewChecker(tokenauth.RedisBloomConfig{
		Enabled:   cfg.Server.TokenAuth.RedisBloom.Enabled,
		URL:       cfg.Server.TokenAuth.RedisBloom.URL,
		Password:  cfg.Server.TokenAuth.RedisBloom.Password,
		KeyPrefix: cfg.Server.TokenAuth.RedisBloom.KeyPrefix,
	})
	if err != nil {
		return nil, fmt.Errorf("configure token auth: %w", err)
	}

	//nolint:contextcheck // Relay callbacks use per-request contexts; the startup context is not applicable here.
	relayProxy, err := relay.NewProxy(
		cfg.Server.Adapter.Relay.BaseURL,
		relay.WithTransport(relay.NewTransport(relayTransportOptions(cfg.Server.Adapter.Relay))),
		relay.WithResponsePolicy(newRelayRateLimitPolicy(
			resolver,
			cfg.Server.Adapter.Relay.RateLimitCooldownTTL,
			cfg.Server.Adapter.Relay.RateLimitRetryAfter,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("configure relay proxy: %w", err)
	}

	codexEntries := service.NewCodexEntryService(resolver, tokenChecker)
	rt := &runtimeState{
		runtimeClient: runtimeClient,
		resolver:      resolver,
		tokenChecker:  tokenChecker,
		relayProxy:    relayProxy,
		codexEntries:  codexEntries,
	}

	tlsRuntime, err := newTLSRuntime(ctx, cfg.Server.HTTP.TLS.TLSReloadConfig())
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("configure tls: %w", err)
	}
	rt.tls = tlsRuntime
	return rt, nil
}

func (rt *runtimeState) Close() {
	if rt == nil {
		return
	}
	rt.codexEntries = nil
	rt.relayProxy = nil
	rt.tokenChecker = nil
	rt.resolver = nil
	rt.runtimeClient = nil
	if rt.tls != nil {
		rt.tls.Close()
		rt.tls = nil
	}
}

func relayTransportOptions(cfg config.AdapterRelay) relay.TransportOptions {
	return relay.TransportOptions{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:     cfg.MaxConnsPerHost,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		DisableKeepAlives:   cfg.DisableKeepAlives,
	}
}
