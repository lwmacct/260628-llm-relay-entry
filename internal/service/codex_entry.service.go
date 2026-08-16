package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/repository"
)

type CodexEntryService struct {
	grants         APITokenGrantResolver
	directiveToken string
	now            apiEntryClock
}

func NewCodexEntryService(grants APITokenGrantResolver, settings APIEntrySettings) (*CodexEntryService, error) {
	settings.DirectiveToken = strings.TrimSpace(settings.DirectiveToken)
	if grants == nil || !strings.HasPrefix(settings.DirectiveToken, "dp.22.remote.") {
		return nil, ErrAPIEntryInvalidSettings
	}
	return &CodexEntryService{
		grants: grants, directiveToken: settings.DirectiveToken, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (s *CodexEntryService) PrepareForward(ctx context.Context, input CodexEntryInput) (CodexPreparedForward, error) {
	token := utilBearerToken(input.Authorization)
	if !utilValidAPIToken(token) {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusUnauthorized, Message: "invalid or unavailable API token"}
	}
	grant, err := s.grants.FetchAPITokenGrant(ctx, repository.APITokenLookup{Token: token, At: s.now()})
	if errors.Is(err, repository.ErrAPITokenNotFound) {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusUnauthorized, Message: "invalid or unavailable API token"}
	}
	if err != nil {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusServiceUnavailable, Message: "API entry is temporarily unavailable", Err: err}
	}
	resolved := CodexResolvedCredential{
		APIKeyID: grant.APIKeyID, UserID: grant.UserID, BindingID: grant.BindingID, VendorCredentialID: grant.VendorCredentialID,
	}
	return CodexPreparedForward{
		Forward: relay.ForwardRequest{
			DirectiveToken: s.directiveToken, VendorCredentialID: grant.VendorCredentialID,
			AffinityKey: utilAPIAffinityKey(token, grant.APIKeyID, input.SessionID),
			RequestID:   input.RequestID, ClientRequestID: input.ClientRequestID,
		},
		Resolved: resolved,
	}, nil
}
