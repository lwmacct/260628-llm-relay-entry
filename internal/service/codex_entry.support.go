package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
)

const (
	CodexResponsesPath           = "/v1/responses"
	CodexHeaderClientRequestID   = "X-Client-Request-Id"
	CodexHeaderSessionID         = "Session-Id"
	CodexHeaderInternalSessionID = "Session_id"
)

var (
	ErrMissingRuntimeBaseURL = errors.New("runtime api base url is required")
	ErrMissingPlanID         = errors.New("plan id is required")
)

type CodexEntryInput struct {
	UserAgent       string
	Authorization   string
	SessionID       string
	RequestID       string
	ClientRequestID string
}

type CodexPreparedForward struct {
	Forward  relay.ForwardRequest
	Resolved CodexResolvedCredential
}

type CodexResolvedCredential struct {
	Payload       relay.CredentialPayload
	ContextID     string
	PoolID        string
	ResourceID    string
	PayloadFields []string
}

type CodexCredentialResolver interface {
	ResolveCredential(ctx context.Context, key string, sessionID string, clientRequestID string) (CodexResolvedCredential, error)
}

type CodexResourceReporter interface {
	ReportResourceCooldown(ctx context.Context, contextID string, reasonCode string, cooldownTTL time.Duration, clientRequestID string) error
}

type CodexEntryError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *CodexEntryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return e.Err.Error()
}

func (e *CodexEntryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsCodexUserAgent(userAgent string) bool {
	normalized := strings.ToLower(strings.TrimSpace(userAgent))
	return normalized == "codex" ||
		strings.HasPrefix(normalized, "codex/") ||
		strings.HasPrefix(normalized, "codex-")
}

func utilBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}
