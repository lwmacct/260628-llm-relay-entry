package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderClientRequestID  = "X-Client-Request-Id"
	HeaderTargetID         = "X-Relay-Target-Id"
	HeaderResolverAffinity = "X-Resolver-Affinity-Key"
	maxErrorBodyBytes      = 256 * 1024
)

type errorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

type ForwardRequest struct {
	DirectiveToken  string
	RelayTargetID   string
	AffinityKey     string
	RequestID       string
	ClientRequestID string
}

type Proxy struct {
	proxy     *httputil.ReverseProxy
	transport http.RoundTripper
}

type forwardRequestContextKey struct{}

type TransportOptions struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	DisableKeepAlives   bool
}

type Option func(*Proxy)

func WithTransport(transport http.RoundTripper) Option {
	return func(proxy *Proxy) { proxy.transport = transport }
}

func NewTransport(opts TransportOptions) http.RoundTripper {
	return NewTransportWithBase(http.DefaultTransport, opts)
}

func NewTransportWithBase(base http.RoundTripper, opts TransportOptions) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		return base
	}
	cloned := transport.Clone()
	if opts.MaxIdleConns > 0 {
		cloned.MaxIdleConns = opts.MaxIdleConns
	}
	if opts.MaxIdleConnsPerHost > 0 {
		cloned.MaxIdleConnsPerHost = opts.MaxIdleConnsPerHost
	}
	if opts.MaxConnsPerHost > 0 {
		cloned.MaxConnsPerHost = opts.MaxConnsPerHost
	}
	if opts.IdleConnTimeout > 0 {
		cloned.IdleConnTimeout = opts.IdleConnTimeout
	}
	cloned.DisableKeepAlives = opts.DisableKeepAlives
	return cloned
}

func NewProxy(baseURL string, options ...Option) (*Proxy, error) {
	targetURL, err := parseAbsoluteURL(baseURL)
	if err != nil {
		return nil, err
	}
	proxy := &Proxy{}
	for _, option := range options {
		if option != nil {
			option(proxy)
		}
	}
	if proxy.transport == nil {
		proxy.transport = http.DefaultTransport
	}
	proxy.proxy = &httputil.ReverseProxy{
		FlushInterval: -1,
		Transport:     proxy.transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			forward, _ := forwardRequestFromContext(request.In.Context())
			rewriteOutboundRequest(request, targetURL)
			request.Out.Header.Set("Authorization", "Bearer "+forward.DirectiveToken)
			request.Out.Header.Set(HeaderTargetID, forward.RelayTargetID)
			request.Out.Header.Set(HeaderResolverAffinity, forward.AffinityKey)
			if forward.ClientRequestID != "" {
				request.Out.Header.Set(HeaderClientRequestID, forward.ClientRequestID)
			}
		},
		ModifyResponse: func(response *http.Response) error {
			if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
				suppressErrorResponse(response)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("Directive proxy request failed", "request_id", requestIDFromContext(r.Context()), "error", err.Error())
			writeError(w, requestIDFromContext(r.Context()), http.StatusBadGateway, "relay request failed")
		},
	}
	return proxy, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, forward ForwardRequest) {
	if p == nil || p.proxy == nil {
		http.NotFound(w, r)
		return
	}
	ctx := context.WithValue(r.Context(), forwardRequestContextKey{}, forward)
	p.proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (p *Proxy) CloseIdleConnections() {
	if closer, ok := p.transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func rewriteOutboundRequest(request *httputil.ProxyRequest, target *url.URL) {
	request.Out.URL.Scheme = target.Scheme
	request.Out.URL.Host = target.Host
	request.Out.URL.Path = joinURLPath(target.Path, request.In.URL.Path)
	request.Out.URL.RawPath = ""
	request.Out.URL.RawQuery = request.In.URL.RawQuery
	request.Out.Host = ""
	stripUntrustedHeaders(request.Out.Header)
}

func stripUntrustedHeaders(header http.Header) {
	for key := range header {
		lower := strings.ToLower(strings.TrimSpace(key))
		if lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" || lower == "cookie2" ||
			lower == "x-proxy-directive" || lower == "m-runtime-key" || lower == "session-id" || lower == "session_id" ||
			strings.HasPrefix(lower, "x-relay-") || lower == strings.ToLower(HeaderResolverAffinity) ||
			strings.HasPrefix(lower, "x-dp-") || strings.HasPrefix(lower, "cf-") || strings.HasPrefix(lower, "x-forwarded-") ||
			lower == "forwarded" || lower == "via" || lower == "x-real-ip" || lower == "true-client-ip" || lower == "cdn-loop" {
			header.Del(key)
		}
	}
}

func suppressErrorResponse(response *http.Response) {
	if response.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
		_ = response.Body.Close()
		slog.Warn("Directive proxy returned non-success", "status", response.StatusCode, "body_bytes", len(raw))
	}
	body, err := json.Marshal(errorResponse{Error: "relay request failed"})
	if err != nil {
		body = []byte(`{"error":"relay request failed"}`)
	}
	body = append(body, '\n')
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header = make(http.Header)
	response.Header.Set("Content-Type", "application/json")
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func forwardRequestFromContext(ctx context.Context) (ForwardRequest, bool) {
	forward, ok := ctx.Value(forwardRequestContextKey{}).(ForwardRequest)
	return forward, ok
}

func requestIDFromContext(ctx context.Context) string {
	forward, _ := forwardRequestFromContext(ctx)
	return strings.TrimSpace(forward.RequestID)
}

func writeError(w http.ResponseWriter, requestID string, statusCode int, message string) {
	raw, err := json.Marshal(errorResponse{Error: message, RequestID: requestID})
	if err != nil {
		raw = []byte(`{"error":"relay request failed"}`)
	}
	raw = append(raw, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(statusCode)
	_, _ = w.Write(raw)
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid relay base URL")
	}
	return parsed, nil
}

func joinURLPath(prefix, suffix string) string {
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(suffix, "/")
}
