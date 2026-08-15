package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/lwmacct/260628-llm-relay-entry/internal/handler"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func newHTTPServer(cfg *config.Config, rt *runtimeState) (*http.Server, error) {
	srv := &http.Server{
		Addr: cfg.Server.HTTP.Listen, Handler: newHTTPHandler(cfg, rt),
		ReadHeaderTimeout: cfg.Server.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.HTTP.ReadTimeout, WriteTimeout: cfg.Server.HTTP.WriteTimeout,
		IdleTimeout: cfg.Server.HTTP.IdleTimeout,
	}
	if rt != nil && rt.tls != nil && rt.tls.config != nil {
		srv.TLSConfig = rt.tls.config
	}
	return srv, nil
}

func newHTTPHandler(cfg *config.Config, rt *runtimeState) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { writeHealth(w, http.StatusOK, "ok") })
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { writeHealth(w, http.StatusOK, "ok") })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if rt == nil || !rt.Ready(r.Context()) {
			writeHealth(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeHealth(w, http.StatusOK, "ready")
	})
	if rt != nil && rt.codexEntries != nil && rt.relayProxy != nil {
		handler.RegisterCodexEntry(mux, rt.codexEntries, rt.relayProxy)
	}
	next := handler.WrapRecovery(mux)
	if cfg.Server.HTTP.EnableDebugRequests {
		next = debugRequestLoggingMiddleware(next)
	}
	return next
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	body := struct {
		Status    string    `json:"status"`
		Timestamp time.Time `json:"timestamp"`
	}{Status: state, Timestamp: time.Now().UTC()}
	raw, err := json.Marshal(body)
	if err != nil {
		http.Error(w, "failed to encode health response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}

func debugRequestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
			slog.Debug("HTTP request received", "request_id", r.Header.Get(service.CodexHeaderClientRequestID), "method", r.Method, "path", r.URL.Path, "content_length", r.ContentLength)
		}
		next.ServeHTTP(w, r)
	})
}

func streamingSafeDefaultTimeouts(cfg *config.Config) {
	if cfg.Server.HTTP.ReadHeaderTimeout <= 0 {
		cfg.Server.HTTP.ReadHeaderTimeout = 10 * time.Second
	}
	if cfg.Server.HTTP.IdleTimeout <= 0 {
		cfg.Server.HTTP.IdleTimeout = time.Minute
	}
}
