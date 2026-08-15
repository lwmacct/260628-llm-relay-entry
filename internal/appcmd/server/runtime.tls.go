package server

import (
	"context"
	"crypto/tls"
	"log/slog"

	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/adapters/op"
	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
)

const httpTLSMinVersion = tls.VersionTLS12

type tlsRuntime struct {
	config *tls.Config
	store  *tlsreload.Store
}

func newTLSRuntime(ctx context.Context, cfg tlsreload.Config) (*tlsRuntime, error) {
	if !cfg.Enabled {
		return &tlsRuntime{}, nil
	}

	store, err := tlsreload.New(
		ctx,
		cfg,
		tlsreload.WithLogger(slog.Default()),
		tlsreload.WithAdapters(op.New(op.Options{})),
	)
	if err != nil {
		return nil, err
	}

	return &tlsRuntime{
		config: &tls.Config{
			MinVersion:     httpTLSMinVersion,
			GetCertificate: store.GetCertificate,
		},
		store: store,
	}, nil
}

func (rt *tlsRuntime) Close() {
	if rt == nil || rt.store == nil {
		return
	}
	_ = rt.store.Close()
	rt.store = nil
}
