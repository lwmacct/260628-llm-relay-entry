package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/config"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func TestNewHTTPServerUsesStreamingSafeTimeouts(t *testing.T) {
	cfg := testConfig()
	rt := &runtimeState{}

	server, err := newHTTPServer(&cfg, rt)
	if err != nil {
		t.Fatalf("new http server: %v", err)
	}

	if server.ReadHeaderTimeout != 10*time.Second {
		t.Fatalf("unexpected read header timeout: %s", server.ReadHeaderTimeout)
	}
	if server.ReadTimeout != 0 {
		t.Fatalf("unexpected read timeout: %s", server.ReadTimeout)
	}
	if server.WriteTimeout != 0 {
		t.Fatalf("unexpected write timeout: %s", server.WriteTimeout)
	}
	if server.IdleTimeout != time.Minute {
		t.Fatalf("unexpected idle timeout: %s", server.IdleTimeout)
	}
}

func TestHTTPHandlerServesHumaHealthAPI(t *testing.T) {
	cfg := testConfig()
	router := newHTTPHandler(&cfg, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/health", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected /api/health 200, got %d body %q", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected health body: %q", resp.Body.String())
	}
}

func TestHTTPHandlerOnlyRoutesExactCodexResponsesPath(t *testing.T) {
	cfg := testConfig()
	router := newHTTPHandler(&cfg, nil)

	for _, path := range []string{"/v1", "/v1/", service.CodexResponsesPath + "/"} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("User-Agent", "codex-tui/1.0.0")
		req.Header.Set("Authorization", "Bearer raw-token")
		req.Header.Set(service.CodexHeaderSessionID, "sess-1")

		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404, got %d body %q", path, resp.Code, resp.Body.String())
		}
	}
}

func TestShouldNotLimitCodexResponsesBody(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	if shouldLimitRequestBody(req) {
		t.Fatal("codex responses endpoint should not use generic API body limit")
	}
}

func TestValidateConfigRequiresRuntimeValues(t *testing.T) {
	cfg := testConfig()
	cfg.Server.Adapter.Runtime.APIBaseURL = ""
	if err := validateConfig(&cfg); err == nil {
		t.Fatal("expected missing runtime api base url error")
	}

	cfg = testConfig()
	cfg.Server.Adapter.Runtime.PlanID = ""
	if err := validateConfig(&cfg); err == nil {
		t.Fatal("expected missing runtime plan id error")
	}
}

func TestValidateHTTPTLS(t *testing.T) {
	cfg := testConfig()
	cfg.Server.HTTP.TLS.Enabled = true
	cfg.Server.HTTP.TLS.CertFile = "cert.pem"
	cfg.Server.HTTP.TLS.KeyFile = ""
	if err := validateConfig(&cfg); err == nil {
		t.Fatal("expected cert without key to be rejected")
	}

	cfg = testConfig()
	cfg.Server.HTTP.TLS.Enabled = true
	cfg.Server.HTTP.TLS.CertFile = "cert.pem"
	cfg.Server.HTTP.TLS.KeyFile = "key.pem"
	cfg.Server.HTTP.TLS.AutoReload = true
	cfg.Server.HTTP.TLS.ReloadInterval = 0
	if err := validateConfig(&cfg); err == nil {
		t.Fatal("expected invalid reload interval to be rejected")
	}
}

func testConfig() config.Config {
	cfg := config.DefaultConfig()
	cfg.Server.Adapter.Relay.BaseURL = "http://127.0.0.1:40047"
	cfg.Server.Adapter.Runtime.APIBaseURL = "http://127.0.0.1:40066"
	cfg.Server.Adapter.Runtime.PlanID = "default"
	cfg.Server.TokenAuth.RedisBloom.Enabled = false
	cfg.Server.HTTP.EnableDebugRequests = false
	return cfg
}
