package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/tokenauth"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func TestCodexEntryHandlerRejectsUnsupportedUserAgentBeforeResolve(t *testing.T) {
	handler, source, gatewayCalled := newRejectingTestHandler(t, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "curl/8.0")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assertRejectedBeforeForwarding(t, resp, http.StatusForbidden, gatewayCalled, source)
}

func TestCodexEntryHandlerRequiresBearerToken(t *testing.T) {
	handler, source, gatewayCalled := newRejectingTestHandler(t, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set(service.CodexHeaderSessionID, "sess-1")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assertRejectedBeforeForwarding(t, resp, http.StatusUnauthorized, gatewayCalled, source)
}

func TestCodexEntryHandlerRejectsDeniedToken(t *testing.T) {
	handler, source, gatewayCalled := newRejectingTestHandler(t, stubTokenChecker{allowed: false})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set(service.CodexHeaderSessionID, "sess-1")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assertRejectedBeforeForwarding(t, resp, http.StatusUnauthorized, gatewayCalled, source)
	if body := resp.Body.String(); !strings.Contains(body, "key is invalid, unavailable, or quota is exhausted") {
		t.Fatalf("expected access denied message, got %q", body)
	}
}

func TestCodexEntryHandlerAcceptsInternalSessionHeader(t *testing.T) {
	assertAcceptedSessionHeader(t, service.CodexHeaderInternalSessionID, "sess-internal")
}

func TestCodexEntryHandlerPrefersHyphenatedSessionHeader(t *testing.T) {
	handler, source := newForwardingTestHandler(t, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set(service.CodexHeaderInternalSessionID, "sess-internal")
	req.Header.Set(service.CodexHeaderSessionID, "sess-hyphen")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if source.lastSessionID != "sess-hyphen" {
		t.Fatalf("unexpected session id: %q", source.lastSessionID)
	}
}

func TestCodexEntryHandlerStripsEdgeProxyHeadersBeforeForwarding(t *testing.T) {
	type capture struct {
		headers http.Header
		path    string
		query   string
	}

	captured := make(chan capture, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- capture{
			headers: r.Header.Clone(),
			path:    r.URL.Path,
			query:   r.URL.RawQuery,
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gateway.Close()

	source := &stubCredentialResolver{payload: mustCredentialPayload(t, map[string]any{"url": gateway.URL})}
	handler := newTestHandler(t, gateway.URL, source, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath+"?beta=true", strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set(service.CodexHeaderSessionID, "sess-1")
	req.Header.Set("Cf-Ray", "ray-123")
	req.Header.Set("Cdn-Loop", "cloudflare")
	req.Header.Set("Forwarded", "for=1.2.3.4;proto=https")
	req.Header.Set("Via", "cloudflare")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-Ip", "1.2.3.4")
	req.Header.Set("X-Proxy-Directive", "client-must-not-control-directive")
	req.Header.Set("X-Custom", "keep-me")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}

	got := <-captured
	if got.path != "/responses" || got.query != "beta=true" {
		t.Fatalf("unexpected forward target path=%q query=%q", got.path, got.query)
	}
	for _, key := range []string{
		"Cf-Ray", "Cdn-Loop", "Forwarded", "Via", "X-Forwarded-For", "X-Real-Ip",
		"X-Proxy-Directive", service.CodexHeaderSessionID, service.CodexHeaderInternalSessionID,
	} {
		if value := got.headers.Get(key); value != "" {
			t.Fatalf("expected %s to be stripped, got %q", key, value)
		}
	}
	if got.headers.Get("X-Custom") != "keep-me" {
		t.Fatalf("expected custom header to be preserved")
	}
	if runtimeKey := got.headers.Get(relay.HeaderRuntimeKey); runtimeKey != "raw-token" {
		t.Fatalf("unexpected runtime key: %q", runtimeKey)
	}
	if got.headers.Get("Authorization") == "Bearer raw-token" {
		t.Fatal("expected outbound authorization to use encoded credential payload")
	}
}

func TestCodexEntryHandlerPassesThrough2xxRelayResponse(t *testing.T) {
	const relayBody = `{"id":"resp-1","status":"created"}`

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Relay-Debug", "relay-header-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(relayBody))
	}))
	defer gateway.Close()

	source := &stubCredentialResolver{payload: mustCredentialPayload(t, map[string]any{"url": gateway.URL})}
	handler := newTestHandler(t, gateway.URL, source, nil, nil)

	req := codexRequest(t)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if body := resp.Body.String(); body != relayBody {
		t.Fatalf("expected relay body to pass through, got %q", body)
	}
	if got := resp.Header().Get("X-Relay-Debug"); got != "relay-header-value" {
		t.Fatalf("expected relay header to pass through, got %q", got)
	}
}

func TestCodexEntryHandlerSuppressesNon2xxRelayResponseBody(t *testing.T) {
	const relayBody = "relay response body"

	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Relay-Debug", "relay-header-value")
		w.Header().Set("Set-Cookie", "session=relay-cookie-value")
		http.Error(w, relayBody, http.StatusUnauthorized)
	}))
	defer gateway.Close()

	source := &stubCredentialResolver{payload: mustCredentialPayload(t, map[string]any{"url": gateway.URL})}
	handler := newTestHandler(t, gateway.URL, source, nil, nil)

	req := codexRequest(t)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	body := resp.Body.String()
	if strings.Contains(body, relayBody) || !strings.Contains(body, "relay: non-2xx response") {
		t.Fatalf("unexpected safe error body: %q", body)
	}
	for _, key := range []string{"X-Relay-Debug", "Set-Cookie"} {
		if value := resp.Header().Get(key); value != "" {
			t.Fatalf("expected %s to be stripped, got %q", key, value)
		}
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}
}

func TestCodexEntryHandlerReportsRelayRateLimit(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "relay rate limit detail", http.StatusTooManyRequests)
	}))
	defer gateway.Close()

	source := &stubCredentialResolver{payload: mustCredentialPayload(t, map[string]any{"url": gateway.URL})}
	reporter := &stubResourceReporter{}
	handler := newTestHandler(t, gateway.URL, source, nil, reporter)

	req := codexRequest(t)
	req.Header.Set(service.CodexHeaderClientRequestID, "client-req-1")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if retryAfter := resp.Header().Get("Retry-After"); retryAfter != "2" {
		t.Fatalf("expected Retry-After 2, got %q", retryAfter)
	}
	var body struct {
		Error struct {
			Code        string `json:"code"`
			Retryable   bool   `json:"retryable"`
			RetryReason string `json:"retry_reason"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response body: %v", err)
	}
	if body.Error.Code != "server_is_overloaded" || !body.Error.Retryable || body.Error.RetryReason != "resource_rate_limited" {
		t.Fatalf("unexpected retryable error body: %s", resp.Body.String())
	}
	if reporter.contextID != "ctx-test" || reporter.clientRequestID != "client-req-1" {
		t.Fatalf("unexpected cooldown report: %#v", reporter)
	}
	if reporter.called.Load() != 1 {
		t.Fatalf("expected reporter called once, got %d", reporter.called.Load())
	}
}

func TestIsCodexUserAgent(t *testing.T) {
	for _, ua := range []string{"codex", "codex/1.0.0", "codex-cli/1.0.0", "codex-tui/1.0.0"} {
		if !service.IsCodexUserAgent(ua) {
			t.Fatalf("expected %q to be recognized as codex user agent", ua)
		}
	}
	for _, ua := range []string{"", "claude-cli/1.0.0", "curl/8.0", "my-codex-client/1.0.0"} {
		if service.IsCodexUserAgent(ua) {
			t.Fatalf("expected %q to be rejected", ua)
		}
	}
}

func assertAcceptedSessionHeader(t *testing.T, headerName string, sessionID string) {
	t.Helper()
	handler, source := newForwardingTestHandler(t, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set(headerName, sessionID)

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if source.lastSessionID != sessionID {
		t.Fatalf("unexpected session id: %q", source.lastSessionID)
	}
}

func codexRequest(t *testing.T) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, service.CodexResponsesPath, strings.NewReader(`{}`))
	req.Header.Set("User-Agent", "codex-tui/1.0.0")
	req.Header.Set("Authorization", "Bearer raw-token")
	req.Header.Set(service.CodexHeaderSessionID, "sess-1")
	return req
}

func newRejectingTestHandler(t *testing.T, tokenChecker tokenauth.Checker) (http.Handler, *stubCredentialResolver, *atomic.Bool) {
	t.Helper()
	gatewayCalled := &atomic.Bool{}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalled.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(gateway.Close)

	source := &stubCredentialResolver{}
	return newTestHandler(t, gateway.URL, source, tokenChecker, nil), source, gatewayCalled
}

func newForwardingTestHandler(t *testing.T, tokenChecker tokenauth.Checker) (http.Handler, *stubCredentialResolver) {
	t.Helper()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(gateway.Close)

	source := &stubCredentialResolver{payload: mustCredentialPayload(t, map[string]any{"url": gateway.URL})}
	return newTestHandler(t, gateway.URL, source, tokenChecker, nil), source
}

func newTestHandler(
	t *testing.T,
	relayBaseURL string,
	source *stubCredentialResolver,
	tokenChecker tokenauth.Checker,
	reporter service.CodexResourceReporter,
) http.Handler {
	t.Helper()
	options := []relay.Option{}
	if reporter != nil {
		options = append(options, relay.WithResponsePolicy(testRateLimitPolicy{reporter: reporter}))
	}
	relayProxy, err := relay.NewProxy(relayBaseURL, options...)
	if err != nil {
		t.Fatalf("new relay proxy: %v", err)
	}
	entries := service.NewCodexEntryService(source, tokenChecker)
	mux := http.NewServeMux()
	RegisterCodexEntry(mux, entries, relayProxy)
	return mux
}

type testRateLimitPolicy struct {
	reporter service.CodexResourceReporter
}

func (p testRateLimitPolicy) HandleRelayResponse(ctx context.Context, resp *http.Response, forward relay.ForwardRequest) *relay.ErrorResponseOverride {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	_ = p.reporter.ReportResourceCooldown(ctx, forward.ContextID, "rate_limited", 5*time.Minute, forward.ClientRequestID)
	return &relay.ErrorResponseOverride{
		StatusCode:  http.StatusServiceUnavailable,
		Message:     "adapter: upstream resource is temporarily unavailable; retry request",
		Code:        "server_is_overloaded",
		Retryable:   true,
		RetryReason: "resource_rate_limited",
		RetryAfter:  "2",
	}
}

func assertRejectedBeforeForwarding(t *testing.T, resp *httptest.ResponseRecorder, wantStatus int, gatewayCalled *atomic.Bool, source *stubCredentialResolver) {
	t.Helper()
	if resp.Code != wantStatus {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if gatewayCalled.Load() {
		t.Fatal("gateway should not have been called")
	}
	if source.called.Load() != 0 {
		t.Fatalf("source should not have been called, got %d", source.called.Load())
	}
}

func mustCredentialPayload(t *testing.T, value map[string]any) relay.CredentialPayload {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	payload, err := relay.DecodeCredentialPayload(raw)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

type stubCredentialResolver struct {
	payload       relay.CredentialPayload
	err           error
	lastKey       string
	lastSessionID string
	called        atomic.Int32
}

func (s *stubCredentialResolver) ResolveCredential(_ context.Context, key string, sessionID string, _ string) (service.CodexResolvedCredential, error) {
	s.lastKey = key
	s.lastSessionID = sessionID
	s.called.Add(1)
	if s.err != nil {
		return service.CodexResolvedCredential{}, s.err
	}
	return service.CodexResolvedCredential{
		Payload:       s.payload,
		ContextID:     "ctx-test",
		PoolID:        "pool-test",
		ResourceID:    "resource-test",
		PayloadFields: relay.CredentialPayloadFieldNames(s.payload),
	}, nil
}

type stubTokenChecker struct {
	allowed bool
	err     error
}

func (s stubTokenChecker) CheckToken(context.Context, string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.allowed, nil
}

type stubResourceReporter struct {
	contextID       string
	reasonCode      string
	cooldownTTL     time.Duration
	clientRequestID string
	err             error
	called          atomic.Int32
}

func (s *stubResourceReporter) ReportResourceCooldown(_ context.Context, contextID string, reasonCode string, cooldownTTL time.Duration, clientRequestID string) error {
	s.contextID = contextID
	s.reasonCode = reasonCode
	s.cooldownTTL = cooldownTTL
	s.clientRequestID = clientRequestID
	s.called.Add(1)
	if s.err != nil {
		return s.err
	}
	return nil
}

func TestCodexEntryHandlerReturnsBadGatewayWhenResolverFails(t *testing.T) {
	gatewayCalled := &atomic.Bool{}
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayCalled.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gateway.Close()

	resolverErr := errors.New("resolved resource payload is empty")
	source := &stubCredentialResolver{err: resolverErr}
	handler := newTestHandler(t, gateway.URL, source, nil, nil)

	req := codexRequest(t)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("unexpected response code: %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), resolverErr.Error()) {
		t.Fatalf("did not expect resolver detail in response body: %q", resp.Body.String())
	}
	if gatewayCalled.Load() {
		t.Fatal("gateway should not have been called")
	}
}
