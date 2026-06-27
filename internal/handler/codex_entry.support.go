package handler

import (
	"net/http"

	"github.com/lwmacct/260628-llm-relay-entry/internal/infra/relay"
)

type codexRelayForwarder interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request, forward relay.ForwardRequest)
}
