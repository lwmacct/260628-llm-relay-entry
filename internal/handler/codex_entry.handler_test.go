package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/repository"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func TestCodexEntryAuthorizesAndForwardsResponses(t *testing.T) {
	token := testAPIToken()
	//nolint:gosec // These UUIDs are non-secret test fixture identifiers.
	grants := &stubGrantResolver{grant: repository.APITokenGrant{
		APIKeyID: 42, UserID: 7, BindingID: "01990f4a-9e4c-7c42-a7ec-5c3f37a6f6b2",
		VendorCredentialID: "01990f4a-9e4c-7c42-a7ec-5c3f37a6f6b3",
	}}
	//nolint:gosec // The values are deliberately invalid test-only credentials.
	entries, err := service.NewCodexEntryService(grants, service.APIEntrySettings{
		DigestKeyID: "v1", DigestKey: "test-digest-key", DirectiveToken: "dp.22.remote.payload.signature",
	})
	if err != nil {
		t.Fatal(err)
	}
	forwarder := new(stubForwarder)
	mux := http.NewServeMux()
	RegisterCodexEntry(mux, entries, forwarder)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/responses?stream=true", strings.NewReader(`{"model":"gpt-test"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", "curl/8.0")
	request.Header.Set(service.CodexHeaderSessionID, "session-a")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if grants.received.DigestKeyID != "v1" || grants.received.TokenDigest == "" {
		t.Fatalf("unexpected digest lookup: %#v", grants.received)
	}
	if forwarder.forward.DirectiveToken != "dp.22.remote.payload.signature" || forwarder.forward.VendorCredentialID != grants.grant.VendorCredentialID || !strings.HasPrefix(forwarder.forward.AffinityKey, "a1_") {
		t.Fatalf("unexpected forward: %#v", forwarder.forward)
	}
}

func TestCodexEntryRejectsInvalidOrUnavailableToken(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		lookup error
	}{
		//nolint:gosec // This malformed value is intentionally not a real token.
		{name: "malformed", token: "not-a-relay-token"},
		{name: "unavailable", token: testAPIToken(), lookup: repository.ErrAPITokenNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			//nolint:gosec // The values are deliberately invalid test-only credentials.
			entries, err := service.NewCodexEntryService(&stubGrantResolver{err: test.lookup}, service.APIEntrySettings{
				DigestKeyID: "v1", DigestKey: "test-digest-key", DirectiveToken: "dp.22.remote.payload.signature",
			})
			if err != nil {
				t.Fatal(err)
			}
			mux := http.NewServeMux()
			RegisterCodexEntry(mux, entries, new(stubForwarder))
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/responses", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCodexEntryReturnsServiceUnavailableForDatabaseFailure(t *testing.T) {
	//nolint:gosec // The values are deliberately invalid test-only credentials.
	entries, err := service.NewCodexEntryService(&stubGrantResolver{err: errors.New("database unavailable")}, service.APIEntrySettings{
		DigestKeyID: "v1", DigestKey: "test-digest-key", DirectiveToken: "dp.22.remote.payload.signature",
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterCodexEntry(mux, entries, new(stubForwarder))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/responses", nil)
	request.Header.Set("Authorization", "Bearer "+testAPIToken())
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func testAPIToken() string {
	return "llmr_v1_" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("\xff", 32)))
}

type stubGrantResolver struct {
	grant    repository.APITokenGrant
	err      error
	received repository.APITokenDigest
}

func (s *stubGrantResolver) FetchAPITokenGrantByDigest(_ context.Context, digest repository.APITokenDigest) (*repository.APITokenGrant, error) {
	s.received = digest
	if s.err != nil {
		return nil, s.err
	}
	value := s.grant
	return &value, nil
}

type stubForwarder struct{ forward relay.ForwardRequest }

func (s *stubForwarder) ServeHTTP(w http.ResponseWriter, _ *http.Request, forward relay.ForwardRequest) {
	s.forward = forward
	w.WriteHeader(http.StatusOK)
}
