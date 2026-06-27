package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

func WrapRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}
			slog.Error(
				"HTTP handler panic",
				"method", utilSanitizeLogValue(r.Method),
				"path", utilSanitizeLogValue(utilRequestPath(r)),
				"request_id", utilSanitizeLogValue(r.Header.Get(service.CodexHeaderClientRequestID)),
				"panic", utilSanitizeLogValue(utilRecoveredValue(recovered)),
				"stack", string(debug.Stack()),
			)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}

func utilRecoveredValue(value any) string {
	if err, ok := value.(error); ok {
		return err.Error()
	}
	if text, ok := value.(string); ok {
		return text
	}
	return "non-string panic value"
}
