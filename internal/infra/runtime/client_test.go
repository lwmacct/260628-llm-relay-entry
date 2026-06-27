package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type capturedRuntimeRequest struct {
	authHeader      string
	clientRequestID string
	contentType     string
	body            map[string]any
}

func TestClientResolveUsesRuntimeHTTPAPI(t *testing.T) {
	captured := make(chan capturedRuntimeRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captureRuntimeRequest(t, r, "/api/runtime/resolve", captured)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"context_id":"ctx-1","expires_at":"2026-05-03T11:53:08Z","resource":{"pool_id":"shell_relay","resource_id":"digitflow","kind":"codex","payload":{"url":"https://api.example.com/v1"}},"reused":false}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:   server.URL,
		AuthToken: "secret-token",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := client.ResolveWithRequestID(context.Background(), ResolveRequest{
		Key:                  "lwmacct-user-token",
		SessionID:            "sess-1",
		PlanID:               "plan-main",
		AllowPartialFailover: true,
	}, "client-req-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.ContextID != "ctx-1" {
		t.Fatalf("unexpected context id: %q", result.ContextID)
	}
	if result.Resource.Kind != "codex" {
		t.Fatalf("unexpected resource kind: %q", result.Resource.Kind)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Resource.Payload, &payload); err != nil {
		t.Fatalf("decode resource payload: %v", err)
	}
	if payload["url"] != "https://api.example.com/v1" {
		t.Fatalf("unexpected resource payload: %#v", payload)
	}

	got := <-captured
	if got.authHeader != "Bearer secret-token" {
		t.Fatalf("unexpected auth header: %q", got.authHeader)
	}
	if got.clientRequestID != "client-req-1" {
		t.Fatalf("unexpected client request id: %q", got.clientRequestID)
	}
	if got.contentType != "application/json" {
		t.Fatalf("unexpected content type: %q", got.contentType)
	}
	if got.body["key"] != "lwmacct-user-token" {
		t.Fatalf("unexpected key: %q", got.body["key"])
	}
	if got.body["session_id"] != "sess-1" {
		t.Fatalf("unexpected session_id: %q", got.body["session_id"])
	}
	if _, ok := got.body["client_session_id"]; ok {
		t.Fatalf("did not expect client_session_id in request body: %#v", got.body)
	}
	if got.body["plan_id"] != "plan-main" {
		t.Fatalf("unexpected plan_id: %q", got.body["plan_id"])
	}
	if got.body["allow_partial_failover"] != true {
		t.Fatal("expected allow_partial_failover to be true")
	}
}

func TestClientReportResourceCooldownUsesRuntimeHTTPAPI(t *testing.T) {
	captured := make(chan capturedRuntimeRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captureRuntimeRequest(t, r, "/api/runtime/contexts/ctx-1/resource/report", captured)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"ctx-1"}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:   server.URL,
		AuthToken: "secret-token",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.ReportResourceCooldownWithRequestID(context.Background(), ReportResourceRequest{
		ContextID:              "ctx-1",
		ReasonCode:             "rate_limited",
		CooldownTTL:            5 * time.Minute,
		CascadeReleaseChildren: true,
	}, "client-req-1")
	if err != nil {
		t.Fatalf("ReportResourceCooldown: %v", err)
	}

	got := <-captured
	if got.authHeader != "Bearer secret-token" {
		t.Fatalf("unexpected auth header: %q", got.authHeader)
	}
	if got.clientRequestID != "client-req-1" {
		t.Fatalf("unexpected client request id: %q", got.clientRequestID)
	}
	if got.contentType != "application/json" {
		t.Fatalf("unexpected content type: %q", got.contentType)
	}
	if got.body["reason_code"] != "rate_limited" {
		t.Fatalf("unexpected reason code: %#v", got.body)
	}
	if got.body["action"] != "cooldown" {
		t.Fatalf("unexpected action: %#v", got.body)
	}
	if got.body["cooldown_ttl"] != float64(300) {
		t.Fatalf("unexpected cooldown ttl: %#v", got.body)
	}
	if got.body["cascade_release_children"] != true {
		t.Fatalf("expected cascade_release_children true: %#v", got.body)
	}
}

func captureRuntimeRequest(
	t *testing.T,
	r *http.Request,
	wantPath string,
	captured chan<- capturedRuntimeRequest,
) {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", r.Method)
	}
	if r.URL.Path != wantPath {
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	captured <- capturedRuntimeRequest{
		authHeader:      r.Header.Get("Authorization"),
		clientRequestID: r.Header.Get("X-Client-Request-Id"),
		contentType:     r.Header.Get("Content-Type"),
		body:            body,
	}
}

func TestClientResolveReturnsAPIError(t *testing.T) {
	const secretFromResponse = "sk-context-chain-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"message":"resolve conflict ` + secretFromResponse + `","code":"failed_precondition"}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Resolve(context.Background(), ResolveRequest{
		Key:       "lwmacct-user-token",
		SessionID: "sess-1",
		PlanID:    "plan-main",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "409 Conflict") {
		t.Fatalf("expected status in error, got %v", err)
	}
	for _, want := range []string{"resolve conflict", "failed_precondition", secretFromResponse} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected context-chain response content %q in error: %v", want, err)
		}
	}
}

func TestDecodeAPIErrorDoesNotUseUpstreamStatusText(t *testing.T) {
	err := decodeAPIError(&http.Response{
		StatusCode: http.StatusConflict,
		Status:     "409 Conflict sk-status-secret",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "sk-status-secret") {
		t.Fatalf("did not expect upstream status text in error: %v", err)
	}
	if !strings.Contains(err.Error(), "409 Conflict") {
		t.Fatalf("expected safe status in error, got %v", err)
	}
}
