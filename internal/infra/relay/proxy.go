package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	responsesPath                 = "/responses"
	HeaderClientRequestID         = "X-Client-Request-Id"
	HeaderRuntimeKey              = "M-Runtime-Key"
	HeaderSessionID               = "Session-Id"
	HeaderInternalSessionID       = "Session_id"
	maxLoggedErrorBodyBytes int64 = 256 * 1024
)

type ForwardRequest struct {
	Payload         CredentialPayload
	RuntimeKey      string
	RequestID       string
	ClientRequestID string
	ContextID       string
	PoolID          string
	ResourceID      string
}

type Proxy struct {
	proxy          *httputil.ReverseProxy
	transport      http.RoundTripper
	responsePolicy ResponsePolicy
}

type forwardRequestContextKey struct{}

type ResponsePolicy interface {
	HandleRelayResponse(ctx context.Context, resp *http.Response, forward ForwardRequest) *ErrorResponseOverride
}

type ErrorResponseOverride struct {
	StatusCode  int
	Message     string
	Code        string
	Retryable   bool
	RetryReason string
	RetryAfter  string
}

type TransportOptions struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	DisableKeepAlives   bool
}

type Option func(*Proxy)

func WithResponsePolicy(policy ResponsePolicy) Option {
	return func(p *Proxy) {
		p.responsePolicy = policy
	}
}

func WithTransport(transport http.RoundTripper) Option {
	return func(p *Proxy) {
		p.transport = transport
	}
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
	proxy.proxy = &httputil.ReverseProxy{
		// Codex responses can stay open for a long time; flush every forwarded
		// chunk immediately instead of relying on generic buffered proxy defaults.
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			rewriteOutboundRequest(pr, targetURL)
			forward, _ := forwardRequestFromContext(pr.In.Context())
			if forward.ClientRequestID != "" {
				pr.Out.Header.Set(HeaderClientRequestID, forward.ClientRequestID)
			}

			encodedPayload, err := EncodeCredentialPayload(forward.Payload)
			if err != nil {
				pr.Out.Header.Del("Authorization")
				pr.Out.Header.Del(HeaderRuntimeKey)
				return
			}

			pr.Out.Header.Set("Authorization", "Bearer "+encodedPayload)
			if forward.RuntimeKey != "" {
				pr.Out.Header.Set(HeaderRuntimeKey, forward.RuntimeKey)
			} else {
				pr.Out.Header.Del(HeaderRuntimeKey)
			}
			pr.Out.Header.Del("X-Proxy-Directive")
			pr.Out.Header.Del(HeaderSessionID)
			pr.Out.Header.Del(HeaderInternalSessionID)
		},
		ModifyResponse: func(resp *http.Response) error {
			logTargetResponseSummary(resp)
			var override *ErrorResponseOverride
			if forward, ok := forwardRequestFromContext(resp.Request.Context()); ok && proxy.responsePolicy != nil {
				override = proxy.responsePolicy.HandleRelayResponse(resp.Request.Context(), resp, forward)
			}
			logAndSuppressNon2xxResponseBody(resp, override)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logProxyFailure(r, err)
			writeError(w, requestIDFromContext(r.Context()), http.StatusBadGateway, "relay: proxy request failed")
		},
		Transport: proxy.transport,
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

func forwardRequestFromContext(ctx context.Context) (ForwardRequest, bool) {
	forward, ok := ctx.Value(forwardRequestContextKey{}).(ForwardRequest)
	return forward, ok
}

func requestIDFromContext(ctx context.Context) string {
	forward, ok := forwardRequestFromContext(ctx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(forward.RequestID)
}

func rewriteOutboundRequest(pr *httputil.ProxyRequest, targetURL *url.URL) {
	pr.Out.URL.Scheme = targetURL.Scheme
	pr.Out.URL.Host = targetURL.Host
	pr.Out.URL.Path = joinURLPath(targetURL.Path, responsesPath)
	pr.Out.URL.RawPath = ""
	pr.Out.URL.RawQuery = pr.In.URL.RawQuery
	pr.Out.URL.ForceQuery = pr.In.URL.ForceQuery
	pr.Out.Host = ""
	stripEdgeProxyHeaders(pr.Out.Header)
}

func stripEdgeProxyHeaders(header http.Header) {
	for key := range header {
		if isEdgeProxyHeader(key) {
			header.Del(key)
		}
	}
}

func isEdgeProxyHeader(key string) bool {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.HasPrefix(lowerKey, "cf-"):
		return true
	case strings.HasPrefix(lowerKey, "x-forwarded-"):
		return true
	default:
		switch lowerKey {
		case "cdn-loop", "true-client-ip", "forwarded", "via", "x-real-ip":
			return true
		default:
			return false
		}
	}
}

func logAndSuppressNon2xxResponseBody(resp *http.Response, override *ErrorResponseOverride) {
	if resp == nil || isSuccessStatus(resp.StatusCode) {
		return
	}

	if override != nil && override.StatusCode > 0 {
		resp.StatusCode = override.StatusCode
		resp.Status = safeHTTPStatus(override.StatusCode)
	}
	retryAfter := resp.Header.Get("Retry-After")
	if override != nil && strings.TrimSpace(override.RetryAfter) != "" {
		retryAfter = strings.TrimSpace(override.RetryAfter)
	}
	var body string
	if resp.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, maxLoggedErrorBodyBytes))
		if err != nil {
			body = "body read failed: " + err.Error()
		} else {
			body = string(raw)
		}
		_ = resp.Body.Close()
	}

	logTargetNonOKResponse(resp, body)

	rawErrorBody, err := json.Marshal(suppressedErrorBody(resp, override))
	if err != nil {
		rawErrorBody = []byte(`{"error":"relay: non-2xx response"}`)
	}
	rawErrorBody = append(rawErrorBody, '\n')

	resp.Body = io.NopCloser(bytes.NewReader(rawErrorBody))
	resp.ContentLength = int64(len(rawErrorBody))
	resp.Header = make(http.Header)
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Content-Length", strconv.Itoa(len(rawErrorBody)))
	if retryAfter != "" {
		resp.Header.Set("Retry-After", retryAfter)
	}
}

func isSuccessStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func suppressedErrorBody(resp *http.Response, override *ErrorResponseOverride) map[string]any {
	body := map[string]any{
		"error": "relay: non-2xx response",
	}
	if requestID := requestIDFromContext(resp.Request.Context()); requestID != "" {
		body["request_id"] = requestID
	}
	if override == nil {
		return body
	}
	message := strings.TrimSpace(override.Message)
	code := strings.TrimSpace(override.Code)
	retryReason := strings.TrimSpace(override.RetryReason)
	if message == "" && code == "" && !override.Retryable && retryReason == "" {
		return body
	}
	if message == "" {
		message = "relay: non-2xx response"
	}
	if retryReason == "" {
		retryReason = "adapter_retryable"
	}
	errorBody := map[string]any{
		"message": message,
	}
	if code != "" {
		errorBody["code"] = code
	}
	if override.Retryable {
		errorBody["retryable"] = true
		errorBody["retry_reason"] = retryReason
	}
	body["error"] = errorBody
	return body
}

func writeError(w http.ResponseWriter, requestID string, statusCode int, message string) {
	body := map[string]any{"error": message}
	if requestID != "" {
		body["request_id"] = requestID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		raw = []byte(`{"error":"adapter: internal error"}`)
	}
	raw = append(raw, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(statusCode)
	_, _ = w.Write(raw)
}

func logProxyFailure(r *http.Request, err error) {
	slog.Warn(
		sanitizeLogValue("Relay request failed"),
		"request_id", sanitizeLogValue(requestIDFromContext(r.Context())),
		"method", sanitizeLogValue(requestMethod(r)),
		"path", sanitizeLogValue(requestPath(r)),
		"error", sanitizeLogValue(errorString(err)),
	)
}

func logTargetNonOKResponse(resp *http.Response, body string) {
	req := resp.Request
	slog.Warn(
		sanitizeLogValue("Relay returned non-2xx response"),
		"request_id", sanitizeLogValue(requestIDFromContext(req.Context())),
		"status", sanitizeLogValue(safeHTTPStatus(resp.StatusCode)),
		"method", sanitizeLogValue(requestMethod(req)),
		"path", sanitizeLogValue(requestPath(req)),
		"body", sanitizeLogValue(body),
	)
}

func logTargetResponseSummary(resp *http.Response) {
	if resp == nil || resp.Request == nil {
		return
	}
	req := resp.Request
	slog.Debug(
		sanitizeLogValue("Relay response received"),
		"request_id", sanitizeLogValue(requestIDFromContext(req.Context())),
		"status", sanitizeLogValue(safeHTTPStatus(resp.StatusCode)),
		"method", sanitizeLogValue(requestMethod(req)),
		"path", sanitizeLogValue(requestPath(req)),
		"content_type", sanitizeLogValue(resp.Header.Get("Content-Type")),
		"content_length", sanitizeLogValue(strconv.FormatInt(resp.ContentLength, 10)),
	)
}

func safeHTTPStatus(statusCode int) string {
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		return strconv.Itoa(statusCode)
	}
	return fmt.Sprintf("%d %s", statusCode, statusText)
}

func requestMethod(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.Method
}

func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sanitizeLogValue(value string) string {
	if value == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n', r == '\r':
			return ' '
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, value)
}

func joinURLPath(prefix string, suffix string) string {
	switch {
	case prefix == "":
		if suffix == "" {
			return "/"
		}
		return suffix
	case suffix == "", suffix == "/":
		return prefix
	case strings.HasSuffix(prefix, "/") && strings.HasPrefix(suffix, "/"):
		return prefix + suffix[1:]
	case !strings.HasSuffix(prefix, "/") && !strings.HasPrefix(suffix, "/"):
		return prefix + "/" + suffix
	default:
		return prefix + suffix
	}
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("url must include scheme and host: %q", raw)
	}
	return parsed, nil
}
