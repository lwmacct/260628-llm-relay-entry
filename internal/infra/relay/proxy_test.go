package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewTransportUsesConfiguredConnectionPool(t *testing.T) {
	base := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		MaxConnsPerHost:     3,
		IdleConnTimeout:     90 * time.Second,
	}

	transport, ok := NewTransportWithBase(base, TransportOptions{
		MaxIdleConns:        4096,
		MaxIdleConnsPerHost: 2048,
		MaxConnsPerHost:     512,
		IdleConnTimeout:     time.Minute,
		DisableKeepAlives:   true,
	}).(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", transport)
	}

	if transport == base {
		t.Fatal("expected cloned transport")
	}
	if transport.MaxIdleConns != 4096 {
		t.Fatalf("unexpected max idle conns: %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 2048 {
		t.Fatalf("unexpected max idle conns per host: %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 512 {
		t.Fatalf("unexpected max conns per host: %d", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout != time.Minute {
		t.Fatalf("unexpected idle conn timeout: %s", transport.IdleConnTimeout)
	}
	if !transport.DisableKeepAlives {
		t.Fatal("expected keep-alives to be disabled")
	}
}

func TestProxyRebuildsTrustedDirectiveHeaders(t *testing.T) {
	transport := &captureTransport{}
	proxy, err := NewProxy("https://directive.internal/base", WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "https://entry.example/v1/responses?stream=true", strings.NewReader("request"))
	request.Header.Set("Authorization", "Bearer user-token")
	request.Header.Set("Cookie", "session=secret")
	request.Header.Set(HeaderTargetRef, "spoofed")
	request.Header.Set(HeaderResolverAffinity, "spoofed")
	request.Header.Set("X-Dp-Internal", "spoofed")
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	response := httptest.NewRecorder()
	//nolint:gosec // These are deliberately invalid test-only credential values.
	proxy.ServeHTTP(response, request, ForwardRequest{
		DirectiveToken: "dp.22.remote.payload.signature", RelayTargetRef: "target-main",
		AffinityKey: "affinity-key", ClientRequestID: "request-id",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	got := transport.request
	if got == nil || got.URL.String() != "https://directive.internal/base/v1/responses?stream=true" {
		t.Fatalf("unexpected target request: %#v", got)
	}
	if got.Header.Get("Authorization") != "Bearer dp.22.remote.payload.signature" || got.Header.Get(HeaderTargetRef) != "target-main" || got.Header.Get(HeaderResolverAffinity) != "affinity-key" {
		t.Fatalf("trusted headers were not rebuilt: %v", got.Header)
	}
	for _, name := range []string{"Cookie", "X-Dp-Internal", "X-Forwarded-For"} {
		if got.Header.Get(name) != "" {
			t.Fatalf("untrusted header %q leaked: %v", name, got.Header)
		}
	}
	if proxy.proxy.FlushInterval != -1 {
		t.Fatalf("streaming flush interval=%s", proxy.proxy.FlushInterval)
	}
}

type captureTransport struct{ request *http.Request }

func (t *captureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.request = request.Clone(request.Context())
	t.request.Header = request.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader("ok")), ContentLength: 2, Request: request,
	}, nil
}

func TestNewProxyUsesConfiguredTransport(t *testing.T) {
	transport := &http.Transport{MaxIdleConnsPerHost: 128}

	proxy, err := NewProxy("http://127.0.0.1:40174", WithTransport(transport))
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	if proxy.proxy.Transport != transport {
		t.Fatalf("expected configured transport, got %T", proxy.proxy.Transport)
	}
}
