package remotetoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateBuildsRelayHTTPRemoteToken(t *testing.T) {
	token, err := Generate("directive-secret", "https://vendor.example/api/relay/resolver", "entry-s2s-token")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 5 || strings.Join(parts[:3], ".") != "dp.22.remote" {
		t.Fatalf("unexpected token: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatal(err)
	}
	var spec remoteSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.HTTP == nil || spec.HTTP.URL != "https://vendor.example/api/relay/resolver" || spec.HTTP.Headers.Mutations[0].Values[0] != "Bearer entry-s2s-token" {
		t.Fatalf("unexpected remote spec: %#v", spec)
	}
	mac := hmac.New(sha256.New, []byte("directive-secret"))
	_, _ = mac.Write([]byte(parts[3]))
	if !hmac.Equal(mac.Sum(nil), mustDecode(t, parts[4])) {
		t.Fatal("invalid token signature")
	}
}

func mustDecode(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
