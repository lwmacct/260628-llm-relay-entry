package relay

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var (
	errEmptyCredentialPayload   = errors.New("resolved resource payload is empty")
	errInvalidCredentialPayload = errors.New("resolved resource payload must be a JSON object")
)

type CredentialPayload struct {
	fields map[string]json.RawMessage
}

func DecodeCredentialPayload(raw json.RawMessage) (CredentialPayload, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return CredentialPayload{}, errEmptyCredentialPayload
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return CredentialPayload{}, fmt.Errorf("%w: %w", errInvalidCredentialPayload, err)
	}
	if fields == nil {
		return CredentialPayload{}, errInvalidCredentialPayload
	}
	if len(fields) == 0 {
		return CredentialPayload{}, errEmptyCredentialPayload
	}
	return CredentialPayload{fields: fields}, nil
}

func EncodeCredentialPayload(payload CredentialPayload) (string, error) {
	raw, err := MarshalCredentialPayload(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func CredentialPayloadFieldNames(payload CredentialPayload) []string {
	if len(payload.fields) == 0 {
		return nil
	}
	names := make([]string, 0, len(payload.fields))
	for name := range payload.fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func MarshalCredentialPayload(payload CredentialPayload) ([]byte, error) {
	fields := cloneRawFields(payload.fields)
	if fields == nil {
		return nil, errEmptyCredentialPayload
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("marshal relay credential payload: %w", err)
	}
	return raw, nil
}

func cloneRawFields(fields map[string]json.RawMessage) map[string]json.RawMessage {
	if len(fields) == 0 {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
