package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func utilSessionID(header http.Header) string {
	if sessionID := strings.TrimSpace(header.Get(service.CodexHeaderSessionID)); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(header.Get(service.CodexHeaderInternalSessionID))
}

func utilEntryStatus(err error) int {
	var entryErr *service.CodexEntryError
	if errors.As(err, &entryErr) && entryErr.StatusCode > 0 {
		return entryErr.StatusCode
	}
	return http.StatusInternalServerError
}

func utilEntryMessage(err error) string {
	var entryErr *service.CodexEntryError
	if errors.As(err, &entryErr) && strings.TrimSpace(entryErr.Message) != "" {
		return strings.TrimSpace(entryErr.Message)
	}
	return "adapter: internal error"
}

func utilWriteError(w http.ResponseWriter, requestID string, statusCode int, message string) {
	raw, err := json.Marshal(utilErrorBody(requestID, message))
	if err != nil {
		raw = []byte(`{"error":"adapter: internal error"}`)
	}
	raw = append(raw, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(statusCode)
	_, _ = w.Write(raw)
}

func utilErrorBody(requestID string, message string) map[string]any {
	body := map[string]any{"error": message}
	if requestID != "" {
		body["request_id"] = requestID
	}
	return body
}

func utilLogResolvedCredential(r *http.Request, requestID string, resolved service.CodexResolvedCredential) {
	slog.Info(
		"Resolved credential",
		"request_id", utilSanitizeLogValue(requestID),
		"method", utilSanitizeLogValue(utilRequestMethod(r)),
		"path", utilSanitizeLogValue(utilRequestPath(r)),
		"context_id", utilSanitizeLogValue(resolved.ContextID),
		"pool_id", utilSanitizeLogValue(resolved.PoolID),
		"resource_id", utilSanitizeLogValue(resolved.ResourceID),
		"payload_fields", utilSanitizeLogValues(resolved.PayloadFields),
	)
}

func utilEnsureRequestID(r *http.Request) string {
	if r == nil {
		return utilNewRequestID()
	}
	if requestID := utilCleanRequestID(r.Header.Get(service.CodexHeaderClientRequestID)); requestID != "" {
		return requestID
	}
	return utilNewRequestID()
}

func utilCleanRequestID(raw string) string {
	return utilSanitizeLogValue(strings.TrimSpace(raw))
}

func utilNewRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

func utilLogUnsupportedUserAgent(r *http.Request, requestID string, userAgent string) {
	if r == nil {
		return
	}
	slog.Warn(
		"Unsupported user agent request rejected",
		"request_id", utilSanitizeLogValue(requestID),
		"method", utilSanitizeLogValue(r.Method),
		"path", utilSanitizeLogValue(utilRequestPath(r)),
		"query", utilSanitizeLogValue(utilRequestQuery(r)),
		"user_agent", utilSanitizeLogValue(userAgent),
		"remote_addr", utilSanitizeLogValue(r.RemoteAddr),
		"content_length", utilSanitizeLogValue(strconv.FormatInt(r.ContentLength, 10)),
		"headers", utilSanitizeHeaders(r.Header),
	)
}

func utilSanitizeHeaders(header http.Header) map[string][]string {
	if len(header) == 0 {
		return nil
	}
	sanitized := make(map[string][]string, len(header))
	for key, values := range header {
		safeKey := utilSanitizeLogValue(key)
		if utilIsSensitiveHeader(key) {
			sanitized[safeKey] = []string{utilSanitizeLogValue("[REDACTED]")}
			continue
		}

		copied := make([]string, len(values))
		for i, value := range values {
			copied[i] = utilSanitizeLogValue(value)
		}
		sanitized[safeKey] = copied
	}
	return sanitized
}

func utilIsSensitiveHeader(key string) bool {
	switch {
	case strings.EqualFold(key, "Authorization"):
		return true
	case strings.EqualFold(key, "Proxy-Authorization"):
		return true
	case strings.EqualFold(key, relay.HeaderRuntimeKey):
		return true
	case strings.EqualFold(key, "Cookie"):
		return true
	case strings.EqualFold(key, "Set-Cookie"):
		return true
	default:
		return false
	}
}

func utilRequestMethod(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.Method
}

func utilRequestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}

func utilRequestQuery(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.RawQuery
}

func utilErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func utilSanitizeLogValue(value string) string {
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

func utilSanitizeLogValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sanitized := make([]string, len(values))
	for i, value := range values {
		sanitized[i] = utilSanitizeLogValue(value)
	}
	return sanitized
}
