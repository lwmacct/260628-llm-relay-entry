package server

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/uptrace/bun"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/database"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/repository"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

type runtimeState struct {
	db           *bun.DB
	relayProxy   *relay.Proxy
	codexEntries *service.CodexEntryService
	tls          *tlsRuntime
	ready        atomic.Bool
}

func newRuntime(ctx context.Context, cfg *config.Config) (*runtimeState, error) {
	db, err := database.Open(ctx, cfg.Server.Database)
	if err != nil {
		return nil, fmt.Errorf("open token database: %w", err)
	}
	entries, err := service.NewCodexEntryService(repository.NewStore(db), service.APIEntrySettings{
		DirectiveToken: cfg.Server.Relay.DirectiveToken,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure API entry: %w", err)
	}
	//nolint:contextcheck // Proxy construction does not perform context-bound work.
	proxy, err := relay.NewProxy(cfg.Server.Relay.BaseURL, relay.WithTransport(relay.NewTransport(relayTransportOptions(cfg.Server.Relay))))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure directive proxy: %w", err)
	}
	rt := &runtimeState{db: db, relayProxy: proxy, codexEntries: entries}
	tlsRuntime, err := newTLSRuntime(ctx, cfg.Server.HTTP.TLS)
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("configure TLS: %w", err)
	}
	rt.tls = tlsRuntime
	rt.ready.Store(true)
	return rt, nil
}

func (rt *runtimeState) Ready(ctx context.Context) bool {
	return rt != nil && rt.ready.Load() && rt.db != nil && rt.db.PingContext(ctx) == nil
}

func (rt *runtimeState) MarkNotReady() {
	if rt != nil {
		rt.ready.Store(false)
	}
}

func (rt *runtimeState) Close() {
	if rt == nil {
		return
	}
	rt.MarkNotReady()
	if rt.relayProxy != nil {
		rt.relayProxy.CloseIdleConnections()
		rt.relayProxy = nil
	}
	rt.codexEntries = nil
	if rt.db != nil {
		_ = rt.db.Close()
		rt.db = nil
	}
	if rt.tls != nil {
		rt.tls.Close()
		rt.tls = nil
	}
}

func relayTransportOptions(cfg config.ServerRelay) relay.TransportOptions {
	return relay.TransportOptions{
		MaxIdleConns: cfg.MaxIdleConns, MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost: cfg.MaxConnsPerHost, IdleConnTimeout: cfg.IdleConnTimeout,
		DisableKeepAlives: cfg.DisableKeepAlives,
	}
}
