package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/repository"
)

const (
	CodexResponsesPath           = "/v1/responses"
	CodexHeaderClientRequestID   = "X-Client-Request-Id"
	CodexHeaderSessionID         = "Session-Id"
	CodexHeaderInternalSessionID = "Session_id"
)

const (
	apiKeyPrefix       = "sk-rdp-v1-" //nolint:gosec // This is a public API key format prefix, not a credential.
	apiKeySuffixLength = 43
	apiKeyAlphabet     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

type CodexEntryInput struct {
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
	APIKeyID      int64
	UserID        int64
	BindingID     string
	VendorRouteID string
}

type APITokenGrantResolver interface {
	FetchAPITokenGrant(context.Context, repository.APITokenLookup) (*repository.APITokenGrant, error)
}

type APIEntrySettings struct {
	DirectiveToken string
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
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *CodexEntryError) Unwrap() error { return e.Err }

var ErrAPIEntryInvalidSettings = errors.New("invalid API entry settings")

type apiEntryClock func() time.Time

func utilBearerToken(value string) string {
	scheme, token, found := strings.Cut(strings.TrimSpace(value), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func utilValidAPIToken(token string) bool {
	if !strings.HasPrefix(token, apiKeyPrefix) {
		return false
	}
	suffix := token[len(apiKeyPrefix):]
	if len(suffix) != apiKeySuffixLength {
		return false
	}
	for i := range suffix {
		if !strings.ContainsRune(apiKeyAlphabet, rune(suffix[i])) {
			return false
		}
	}
	return true
}

func utilAPIAffinityKey(secret string, apiKeyID int64, sessionID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("affinity\x00" + strconv.FormatInt(apiKeyID, 10) + "\x00" + strings.TrimSpace(sessionID)))
	return "a1_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
