package handler

import (
	"log/slog"
	"net/http"

	"github.com/lwmacct/260628-llm-relay-entry/internal/service"
)

type codexEntryHandler struct {
	entries *service.CodexEntryService
	relay   codexRelayForwarder
}

func RegisterCodexEntry(mux *http.ServeMux, entries *service.CodexEntryService, relay codexRelayForwarder) {
	handler := codexEntryHandler{
		entries: entries,
		relay:   relay,
	}
	mux.HandleFunc(service.CodexResponsesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		handler.serveHTTP(w, r)
	})
}

func (h codexEntryHandler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if h.entries == nil || h.relay == nil {
		http.NotFound(w, r)
		return
	}
	if r.URL == nil || r.URL.Path != service.CodexResponsesPath {
		http.NotFound(w, r)
		return
	}

	requestID := utilEnsureRequestID(r)
	clientRequestID := utilCleanRequestID(r.Header.Get(service.CodexHeaderClientRequestID))
	if clientRequestID != "" {
		w.Header().Set(service.CodexHeaderClientRequestID, clientRequestID)
	}

	input := service.CodexEntryInput{
		Authorization:   r.Header.Get("Authorization"),
		SessionID:       utilSessionID(r.Header),
		RequestID:       requestID,
		ClientRequestID: clientRequestID,
	}
	prepared, err := h.entries.PrepareForward(r.Context(), input)
	if err != nil {
		h.logPrepareError(r, requestID, err)
		utilWriteError(w, requestID, utilEntryStatus(err), utilEntryMessage(err))
		return
	}
	utilLogResolvedCredential(r, requestID, prepared.Resolved)

	h.relay.ServeHTTP(w, r, prepared.Forward)
}

func (h codexEntryHandler) logPrepareError(r *http.Request, requestID string, err error) {
	status := utilEntryStatus(err)
	if status >= http.StatusInternalServerError {
		slog.Warn(
			"API entry request failed",
			"request_id", utilSanitizeLogValue(requestID),
			"method", utilSanitizeLogValue(utilRequestMethod(r)),
			"path", utilSanitizeLogValue(utilRequestPath(r)),
			"error", utilSanitizeLogValue(utilErrorString(err)),
		)
	}
}
