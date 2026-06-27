package server

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

const relayRateLimitReasonCode = "rate_limited"

type relayRateLimitPolicy struct {
	reporter    service.CodexResourceReporter
	cooldownTTL time.Duration
	retryAfter  time.Duration
}

func newRelayRateLimitPolicy(
	reporter service.CodexResourceReporter,
	cooldownTTL time.Duration,
	retryAfter time.Duration,
) relayRateLimitPolicy {
	if cooldownTTL <= 0 {
		cooldownTTL = 5 * time.Minute
	}
	if retryAfter <= 0 {
		retryAfter = 2 * time.Second
	}
	return relayRateLimitPolicy{
		reporter:    reporter,
		cooldownTTL: cooldownTTL,
		retryAfter:  retryAfter,
	}
}

func (p relayRateLimitPolicy) HandleRelayResponse(
	ctx context.Context,
	resp *http.Response,
	forward relay.ForwardRequest,
) *relay.ErrorResponseOverride {
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if p.reporter == nil {
		logRelayRateLimitReportSkipped(forward, "resource reporter is not configured")
		return p.rateLimitOverride()
	}
	if forward.ContextID == "" {
		logRelayRateLimitReportSkipped(forward, "context id is empty")
		return p.rateLimitOverride()
	}

	if err := p.reporter.ReportResourceCooldown(ctx, forward.ContextID, relayRateLimitReasonCode, p.cooldownTTL, forward.ClientRequestID); err != nil {
		logRelayRateLimitReportFailure(forward, err)
	} else {
		logRelayRateLimitReported(forward, p.cooldownTTL)
	}
	return p.rateLimitOverride()
}

func (p relayRateLimitPolicy) rateLimitOverride() *relay.ErrorResponseOverride {
	return &relay.ErrorResponseOverride{
		StatusCode:  http.StatusServiceUnavailable,
		Message:     "adapter: upstream resource is temporarily unavailable; retry request",
		Code:        "server_is_overloaded",
		Retryable:   true,
		RetryReason: "resource_rate_limited",
		RetryAfter:  strconv.FormatInt(retryAfterSeconds(p.retryAfter), 10),
	}
}

func retryAfterSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 1
	}
	seconds := duration / time.Second
	if duration%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return int64(seconds)
}

func logRelayRateLimitReported(forward relay.ForwardRequest, cooldownTTL time.Duration) {
	slog.Warn(
		"Relay rate limit reported to runtime",
		"request_id", sanitizeLogValue(forward.RequestID),
		"context_id", sanitizeLogValue(forward.ContextID),
		"pool_id", sanitizeLogValue(forward.PoolID),
		"resource_id", sanitizeLogValue(forward.ResourceID),
		"cooldown_ttl", sanitizeLogValue(cooldownTTL.String()),
	)
}

func logRelayRateLimitReportFailure(forward relay.ForwardRequest, err error) {
	slog.Warn(
		"Relay rate limit report failed",
		"request_id", sanitizeLogValue(forward.RequestID),
		"context_id", sanitizeLogValue(forward.ContextID),
		"pool_id", sanitizeLogValue(forward.PoolID),
		"resource_id", sanitizeLogValue(forward.ResourceID),
		"error", sanitizeLogValue(errorString(err)),
	)
}

func logRelayRateLimitReportSkipped(forward relay.ForwardRequest, reason string) {
	slog.Warn(
		"Relay rate limit report skipped",
		"request_id", sanitizeLogValue(forward.RequestID),
		"context_id", sanitizeLogValue(forward.ContextID),
		"pool_id", sanitizeLogValue(forward.PoolID),
		"resource_id", sanitizeLogValue(forward.ResourceID),
		"reason", sanitizeLogValue(reason),
	)
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
