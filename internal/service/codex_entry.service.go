package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/tokenauth"
)

type CodexEntryService struct {
	resolver     CodexCredentialResolver
	tokenChecker tokenauth.Checker
}

func NewCodexEntryService(resolver CodexCredentialResolver, tokenChecker tokenauth.Checker) *CodexEntryService {
	if tokenChecker == nil {
		tokenChecker = tokenauth.NoopChecker{}
	}
	return &CodexEntryService{
		resolver:     resolver,
		tokenChecker: tokenChecker,
	}
}

func (s *CodexEntryService) PrepareForward(ctx context.Context, input CodexEntryInput) (CodexPreparedForward, error) {
	if s == nil || s.resolver == nil {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusNotFound, Message: "adapter: unavailable"}
	}

	if !IsCodexUserAgent(input.UserAgent) {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusForbidden, Message: "adapter: unsupported user agent"}
	}

	rawToken := utilBearerToken(input.Authorization)
	if rawToken == "" {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusUnauthorized, Message: "adapter: missing bearer token"}
	}

	tokenAllowed, err := s.tokenChecker.CheckToken(ctx, rawToken)
	if err != nil {
		return CodexPreparedForward{}, &CodexEntryError{
			StatusCode: http.StatusBadGateway,
			Message:    "adapter: token authorization check failed",
			Err:        fmt.Errorf("token authorization check failed: %w", err),
		}
	}
	if !tokenAllowed {
		return CodexPreparedForward{}, &CodexEntryError{
			StatusCode: http.StatusUnauthorized,
			Message:    "adapter: access denied; key is invalid, unavailable, or quota is exhausted",
		}
	}

	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusBadRequest, Message: "adapter: missing session-id"}
	}

	resolved, err := s.resolver.ResolveCredential(ctx, rawToken, sessionID, input.ClientRequestID)
	if err != nil {
		return CodexPreparedForward{}, &CodexEntryError{
			StatusCode: http.StatusBadGateway,
			Message:    "runtime: resolve credential failed",
			Err:        fmt.Errorf("resolve credential failed: %w", err),
		}
	}

	return CodexPreparedForward{
		Forward: relay.ForwardRequest{
			Payload:         resolved.Payload,
			RuntimeKey:      rawToken,
			RequestID:       input.RequestID,
			ClientRequestID: input.ClientRequestID,
			ContextID:       resolved.ContextID,
			PoolID:          resolved.PoolID,
			ResourceID:      resolved.ResourceID,
		},
		Resolved: resolved,
	}, nil
}
