package relay

import (
	"net/http"
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
