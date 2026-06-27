package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/runtime"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

type runtimeCredentialResolver struct {
	client               *runtime.Client
	planID               string
	allowPartialFailover bool
}

func newRuntimeCredentialResolver(
	client *runtime.Client,
	planID string,
	allowPartialFailover bool,
) (*runtimeCredentialResolver, error) {
	if client == nil {
		return nil, service.ErrMissingRuntimeBaseURL
	}
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return nil, service.ErrMissingPlanID
	}
	return &runtimeCredentialResolver{
		client:               client,
		planID:               planID,
		allowPartialFailover: allowPartialFailover,
	}, nil
}

func (s *runtimeCredentialResolver) ResolveCredential(
	ctx context.Context,
	key string,
	sessionID string,
	clientRequestID string,
) (service.CodexResolvedCredential, error) {
	result, err := s.client.ResolveWithRequestID(ctx, runtime.ResolveRequest{
		Key:                  key,
		SessionID:            sessionID,
		PlanID:               s.planID,
		AllowPartialFailover: s.allowPartialFailover,
	}, clientRequestID)
	if err != nil {
		return service.CodexResolvedCredential{}, err
	}

	payload, err := relay.DecodeCredentialPayload(result.Resource.Payload)
	if err != nil {
		return service.CodexResolvedCredential{}, fmt.Errorf("decode resolved credential payload: %w", err)
	}
	return service.CodexResolvedCredential{
		Payload:       payload,
		ContextID:     result.ContextID,
		PoolID:        result.Resource.PoolID,
		ResourceID:    result.Resource.ResourceID,
		PayloadFields: relay.CredentialPayloadFieldNames(payload),
	}, nil
}

func (s *runtimeCredentialResolver) ReportResourceCooldown(
	ctx context.Context,
	contextID string,
	reasonCode string,
	cooldownTTL time.Duration,
	clientRequestID string,
) error {
	return s.client.ReportResourceCooldownWithRequestID(ctx, runtime.ReportResourceRequest{
		ContextID:              contextID,
		ReasonCode:             reasonCode,
		CooldownTTL:            cooldownTTL,
		CascadeReleaseChildren: true,
	}, clientRequestID)
}
