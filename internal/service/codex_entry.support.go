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
	APIKeyID           int64
	UserID             int64
	BindingID          string
	VendorCredentialID string
}

type APITokenGrantResolver interface {
	FetchAPITokenGrantByDigest(context.Context, repository.APITokenDigest) (*repository.APITokenGrant, error)
}

type APIEntrySettings struct {
	DigestKeyID    string
	DigestKey      string
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

func utilValidAPIToken(token, digestKeyID string) bool {
	prefix := "llmr_" + digestKeyID + "_"
	encoded, found := strings.CutPrefix(token, prefix)
	if !found {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(raw) == 32
}

func utilAPITokenDigest(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func utilAPIAffinityKey(secret string, apiKeyID int64, sessionID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("affinity\x00" + strconv.FormatInt(apiKeyID, 10) + "\x00" + strings.TrimSpace(sessionID)))
	return "a1_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
