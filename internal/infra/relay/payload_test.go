package relay

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeCredentialPayload(t *testing.T) {
	payload, err := DecodeCredentialPayload(rawJSON(`{
		"v": 1,
		"url": "https://api.example.com/v1",
		"headers": [
			{"op": "=", "name": "Authorization", "values": ["Bearer upstream-token"]}
		],
		"labels": {"trace_id": "trace-123"}
	}`))
	if err != nil {
		t.Fatalf("DecodeCredentialPayload: %v", err)
	}
	if string(payload.fields["url"]) != `"https://api.example.com/v1"` {
		t.Fatalf("unexpected payload fields: %#v", payload.fields)
	}
	if _, ok := payload.fields["headers"]; !ok {
		t.Fatalf("expected raw headers to be preserved: %#v", payload.fields)
	}
}

func TestDecodeCredentialPayloadRequiresPayloadObject(t *testing.T) {
	for _, payload := range []json.RawMessage{nil, rawJSON(`null`), rawJSON(`{}`), rawJSON(`[]`)} {
		_, err := DecodeCredentialPayload(payload)
		if err == nil {
			t.Fatalf("expected payload %s to be rejected", payload)
		}
	}
}

func TestEncodeCredentialPayloadPreservesOpaqueFields(t *testing.T) {
	payload, err := DecodeCredentialPayload(rawJSON(`{
		"v": 1,
		"url": "https://api.example.com/v1",
		"key": "schema-owned-by-directive-proxy",
		"headers": [
			{"op": "=", "name": "Authorization", "values": ["Bearer upstream-token"]}
		],
		"future_field": {"nested": true}
	}`))
	if err != nil {
		t.Fatalf("DecodeCredentialPayload: %v", err)
	}

	encoded, err := EncodeCredentialPayload(payload)
	if err != nil {
		t.Fatalf("EncodeCredentialPayload: %v", err)
	}

	decoded := decodeEncodedPayload(t, encoded)
	if decoded["key"] != "schema-owned-by-directive-proxy" {
		t.Fatalf("expected opaque key field to be preserved: %#v", decoded)
	}
	if _, ok := decoded["future_field"].(map[string]any); !ok {
		t.Fatalf("expected future field to be preserved: %#v", decoded["future_field"])
	}
}

func rawJSON(raw string) json.RawMessage {
	return json.RawMessage(raw)
}

func decodeEncodedPayload(t *testing.T, encoded string) map[string]any {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode encoded payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal encoded payload: %v", err)
	}
	return decoded
}
