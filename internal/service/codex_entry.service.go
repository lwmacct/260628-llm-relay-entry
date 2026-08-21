package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/pkg/directive"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/repository"
)

type CodexEntryService struct {
	grants     APITokenGrantResolver
	hmacSecret string
	now        apiEntryClock
}

func NewCodexEntryService(grants APITokenGrantResolver, settings APIEntrySettings) (*CodexEntryService, error) {
	settings.HMACSecret = strings.TrimSpace(settings.HMACSecret)
	if grants == nil || settings.HMACSecret == "" {
		return nil, ErrAPIEntryInvalidSettings
	}
	return &CodexEntryService{
		grants: grants, hmacSecret: settings.HMACSecret, now: func() time.Time { return time.Now().UTC() },
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
	remoteSpec, err := directive.DecodeRemoteSpec([]byte(grant.RemoteSpecJSON))
	if err != nil {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusServiceUnavailable, Message: "API entry remote directive is unavailable", Err: err}
	}
	directiveToken, err := directive.EncodeRemote(s.hmacSecret, remoteSpec)
	if err != nil {
		return CodexPreparedForward{}, &CodexEntryError{StatusCode: http.StatusServiceUnavailable, Message: "API entry remote directive is unavailable", Err: err}
	}
	resolved := CodexResolvedCredential{APIKeyID: grant.APIKeyID, UserID: grant.UserID}
	return CodexPreparedForward{
		Forward: relay.ForwardRequest{
			DirectiveToken: directiveToken,
			AffinityKey:    utilAPIAffinityKey(token, grant.APIKeyID, input.SessionID),
			RequestID:      input.RequestID, ClientRequestID: input.ClientRequestID,
		},
		Resolved: resolved,
	}, nil
}
