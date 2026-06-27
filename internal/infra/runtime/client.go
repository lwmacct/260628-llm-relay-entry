package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	AuthToken      string
	ResolveTimeout time.Duration
	ReportTimeout  time.Duration
}

type Client struct {
	httpClient     *http.Client
	baseURL        *url.URL
	resolveURL     string
	authToken      string
	resolveTimeout time.Duration
	reportTimeout  time.Duration
}

type ResolveRequest struct {
	Key                  string `json:"key"`
	SessionID            string `json:"session_id"`
	PlanID               string `json:"plan_id"`
	AllowPartialFailover bool   `json:"allow_partial_failover"`
}

type ReportResourceRequest struct {
	ContextID              string
	ReasonCode             string
	CooldownTTL            time.Duration
	CascadeReleaseChildren bool
}

type ResolveResult struct {
	ContextID string           `json:"context_id"`
	Resource  ResolvedResource `json:"resource"`
	Reused    bool             `json:"reused"`
}

type ResolvedResource struct {
	PoolID     string          `json:"pool_id"`
	ResourceID string          `json:"resource_id"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
}

type resolveEnvelope struct {
	Data ResolveResult `json:"data"`
}

type errorEnvelope struct {
	Error *errorBody `json:"error"`
}

type errorBody struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func NewClient(cfg Config) (*Client, error) {
	baseURL, err := parseAbsoluteURL(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse context-chain api url: %w", err)
	}

	timeout := cfg.ResolveTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reportTimeout := cfg.ReportTimeout
	if reportTimeout <= 0 {
		reportTimeout = 2 * time.Second
	}

	return &Client{
		httpClient:     &http.Client{},
		baseURL:        baseURL,
		resolveURL:     resolveURL(baseURL).String(),
		authToken:      strings.TrimSpace(cfg.AuthToken),
		resolveTimeout: timeout,
		reportTimeout:  reportTimeout,
	}, nil
}

func (c *Client) Resolve(ctx context.Context, reqBody ResolveRequest) (*ResolveResult, error) {
	return c.ResolveWithRequestID(ctx, reqBody, "")
}

func (c *Client) ResolveWithRequestID(ctx context.Context, reqBody ResolveRequest, requestID string) (*ResolveResult, error) {
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.resolveTimeout)
	defer cancel()

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal context-chain resolve request: %w", err)
	}

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.resolveURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build context-chain resolve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if requestID = sanitizeHeaderValue(requestID); requestID != "" {
		req.Header.Set("X-Client-Request-Id", requestID)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call context-chain resolve api: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, decodeAPIError(resp)
	}

	var responseBody resolveEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return nil, fmt.Errorf("decode context-chain resolve response: %w", err)
	}
	return &responseBody.Data, nil
}

func (c *Client) ReportResourceCooldown(
	ctx context.Context,
	reqBody ReportResourceRequest,
) error {
	return c.ReportResourceCooldownWithRequestID(ctx, reqBody, "")
}

func (c *Client) ReportResourceCooldownWithRequestID(
	ctx context.Context,
	reqBody ReportResourceRequest,
	requestID string,
) error {
	contextID := strings.TrimSpace(reqBody.ContextID)
	if contextID == "" {
		return errors.New("context-chain report resource request requires context_id")
	}
	if reqBody.CooldownTTL <= 0 {
		return errors.New("context-chain report resource request requires positive cooldown ttl")
	}
	cooldownSeconds := int64(reqBody.CooldownTTL / time.Second)
	if reqBody.CooldownTTL%time.Second != 0 {
		cooldownSeconds++
	}
	if cooldownSeconds < 1 {
		cooldownSeconds = 1
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.reportTimeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{
		"reason_code":              strings.TrimSpace(reqBody.ReasonCode),
		"action":                   "cooldown",
		"cooldown_ttl":             cooldownSeconds,
		"cascade_release_children": reqBody.CascadeReleaseChildren,
	})
	if err != nil {
		return fmt.Errorf("marshal context-chain report request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		callCtx,
		http.MethodPost,
		reportResourceURL(c.baseURL, contextID).String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build context-chain report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if requestID = sanitizeHeaderValue(requestID); requestID != "" {
		req.Header.Set("X-Client-Request-Id", requestID)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call context-chain report api: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(resp)
	}
	return nil
}

func sanitizeHeaderValue(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r':
			return -1
		default:
			return r
		}
	}, strings.TrimSpace(value))
}

func resolveURL(baseURL *url.URL) *url.URL {
	resolved := *baseURL
	resolved.Path = joinURLPath(baseURL.Path, "/api/runtime/resolve")
	resolved.RawPath = ""
	resolved.RawQuery = ""
	resolved.ForceQuery = false
	return &resolved
}

func reportResourceURL(baseURL *url.URL, contextID string) *url.URL {
	resolved := *baseURL
	resolved.Path = joinURLPath(baseURL.Path, "/api/runtime/contexts/"+url.PathEscape(contextID)+"/resource/report")
	resolved.RawPath = ""
	resolved.RawQuery = ""
	resolved.ForceQuery = false
	return &resolved
}

func decodeAPIError(resp *http.Response) error {
	if resp.Body == nil {
		return fmt.Errorf("context-chain resolve api returned %s", safeHTTPStatus(resp.StatusCode))
	}

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		return fmt.Errorf("context-chain resolve api returned %s and body read failed: %w", safeHTTPStatus(resp.StatusCode), readErr)
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
		message := strings.TrimSpace(envelope.Error.Message)
		code := strings.TrimSpace(envelope.Error.Code)
		switch {
		case message != "" && code != "":
			return fmt.Errorf("context-chain resolve api returned %s: %s (%s)", safeHTTPStatus(resp.StatusCode), message, code)
		case message != "":
			return fmt.Errorf("context-chain resolve api returned %s: %s", safeHTTPStatus(resp.StatusCode), message)
		}
	}

	message := strings.TrimSpace(string(body))
	if message != "" {
		return fmt.Errorf("context-chain resolve api returned %s: %s", safeHTTPStatus(resp.StatusCode), message)
	}
	return fmt.Errorf("context-chain resolve api returned %s", safeHTTPStatus(resp.StatusCode))
}

func safeHTTPStatus(statusCode int) string {
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		return strconv.Itoa(statusCode)
	}
	return fmt.Sprintf("%d %s", statusCode, statusText)
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
