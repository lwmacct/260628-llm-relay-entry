package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/lwmacct/260628-llm-relay-entry/internal/handler"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

const httpAPIPrefix = "/api"

func newHTTPServer(cfg *config.Config, rt *runtimeState) (*http.Server, error) {
	httpCfg := cfg.Server.HTTP
	srv := &http.Server{
		Addr:              httpCfg.Listen,
		Handler:           newHTTPHandler(cfg, rt),
		ReadHeaderTimeout: httpCfg.ReadHeaderTimeout,
		ReadTimeout:       httpCfg.ReadTimeout,
		WriteTimeout:      httpCfg.WriteTimeout,
		IdleTimeout:       httpCfg.IdleTimeout,
	}
	if rt != nil && rt.tls != nil && rt.tls.config != nil {
		srv.TLSConfig = rt.tls.config
	}
	return srv, nil
}

func newHTTPHandler(cfg *config.Config, rt *runtimeState) http.Handler {
	mux := http.NewServeMux()
	apiHandler := limitRequestBody(handler.NewEndpoint().Handler(), cfg.Server.HTTP.MaxAPIBodyBytes)
	mux.Handle(httpAPIPrefix+"/", http.StripPrefix(httpAPIPrefix, apiHandler))
	registerLegacyHealth(mux)
	if rt != nil && rt.codexEntries != nil && rt.relayProxy != nil {
		handler.RegisterCodexEntry(mux, rt.codexEntries, rt.relayProxy)
	}
	mux.HandleFunc("/v1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})
	if cfg.Server.HTTP.WebRoot != "" {
		mux.Handle("/", http.FileServer(http.Dir(cfg.Server.HTTP.WebRoot)))
	}

	var next http.Handler = mux
	next = handler.WrapRecovery(next)
	if cfg.Server.HTTP.EnableDebugRequests {
		next = debugRequestLoggingMiddleware(next)
	}
	return next
}

func registerLegacyHealth(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		raw, err := json.Marshal(map[string]any{
			"status":    "ok",
			"timestamp": time.Now().UTC(),
		})
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(append(raw, '\n'))
	})
}

func limitRequestBody(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if maxBytes > 0 && shouldLimitRequestBody(r) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func shouldLimitRequestBody(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Body == nil || r.Body == http.NoBody {
		return false
	}
	if r.URL != nil && r.URL.Path == service.CodexResponsesPath {
		return false
	}
	for _, value := range r.Header.Values("Upgrade") {
		if strings.EqualFold(strings.TrimSpace(value), "websocket") {
			return false
		}
	}
	return true
}

func debugRequestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := slog.Default()
		if logger.Enabled(r.Context(), slog.LevelDebug) {
			path := ""
			query := ""
			if r.URL != nil {
				path = r.URL.Path
				query = r.URL.RawQuery
			}
			logger.Debug(
				"HTTP request received",
				"request_id", r.Header.Get(service.CodexHeaderClientRequestID),
				"method", r.Method,
				"path", path,
				"query", query,
				"user_agent", r.UserAgent(),
				"remote_addr", r.RemoteAddr,
				"content_length", r.ContentLength,
			)
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
