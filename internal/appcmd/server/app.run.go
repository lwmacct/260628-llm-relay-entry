package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
)

func (app *App) Run(ctx context.Context) error {
	if err := validateConfig(app.cfg); err != nil {
		return err
	}
	streamingSafeDefaultTimeouts(app.cfg)

	rt, err := newRuntime(ctx, app.cfg)
	if err != nil {
		return err
	}
	defer rt.Close()

	srv, err := newHTTPServer(app.cfg, rt)
	if err != nil {
		return err
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	errCh := make(chan error, 1)
	go func() {
		httpCfg := app.cfg.Server.HTTP
		slog.Info("API entry service starting", "listen", srv.Addr, "https", httpCfg.TLS.Enabled)
		var serveErr error
		if httpCfg.TLS.Enabled {
			serveErr = srv.ServeTLS(ln, "", "")
		} else {
			serveErr = srv.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
		return shutdown(ctx, srv, rt, app.cfg)
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig.String())
		return shutdown(ctx, srv, rt, app.cfg)
	case err := <-errCh:
		return err
	}
}

func shutdown(ctx context.Context, srv *http.Server, rt *runtimeState, cfg *config.Config) error {
	rt.MarkNotReady()
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Server.HTTP.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	slog.Info("web service stopped")
	return nil
}
