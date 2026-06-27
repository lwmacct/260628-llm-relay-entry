package tokenauth

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestTokenHashUsesSHA256Hex(t *testing.T) {
	got := tokenHash("lwmacct-raw-token")
	want := "bcdb8995f52f9a7a99ba5751fc2c12eeb41a048942cfadbdeef68319821b8976"
	if got != want {
		t.Fatalf("unexpected token hash: got %q want %q", got, want)
	}
}

func TestNewTokenCheckerDisabledUsesNoop(t *testing.T) {
	checker, err := NewChecker(RedisBloomConfig{})
	if err != nil {
		t.Fatalf("new token checker: %v", err)
	}

	allowed, err := checker.CheckToken(t.Context(), "any-token")
	if err != nil {
		t.Fatalf("check token: %v", err)
	}
	if !allowed {
		t.Fatal("disabled checker should allow tokens")
	}
}

func TestNewTokenCheckerRequiresRedisConfigWhenEnabled(t *testing.T) {
	_, err := NewChecker(RedisBloomConfig{Enabled: true})
	if err == nil {
		t.Fatal("expected missing redis url error")
	}

	_, err = NewChecker(RedisBloomConfig{
		Enabled: true,
		URL:     "redis://localhost:6379/0",
	})
	if err == nil {
		t.Fatal("expected missing key prefix error")
	}
}

func TestRedisBloomTokenCheckerChecksActiveBloomFilterWithTokenHash(t *testing.T) {
	fakeClient := &stubRedisBloomClient{
		doResult: redis.NewCmdResult(int64(1), nil),
	}
	checker := &redisBloomTokenChecker{
		client:    fakeClient,
		keyPrefix: "token:white",
	}

	allowed, err := checker.CheckToken(t.Context(), "lwmacct-raw-token")
	if err != nil {
		t.Fatalf("check token: %v", err)
	}
	if !allowed {
		t.Fatal("expected token to be allowed")
	}

	wantArgs := []any{
		"EVAL",
		tokenAuthCheckScript,
		2,
		"token:white:active",
		"token:white:bf:",
		"bcdb8995f52f9a7a99ba5751fc2c12eeb41a048942cfadbdeef68319821b8976",
	}
	if !equalRedisArgs(fakeClient.doArgs, wantArgs) {
		t.Fatalf("unexpected EVAL args: got %#v want %#v", fakeClient.doArgs, wantArgs)
	}
}

func TestRedisBloomTokenCheckerRejectsWhenBloomMisses(t *testing.T) {
	checker := &redisBloomTokenChecker{
		client: &stubRedisBloomClient{
			doResult: redis.NewCmdResult(int64(0), nil),
		},
		keyPrefix: "token:white",
	}

	allowed, err := checker.CheckToken(t.Context(), "raw-token")
	if err != nil {
		t.Fatalf("check token: %v", err)
	}
	if allowed {
		t.Fatal("expected token to be rejected")
	}
}

func TestRedisBloomTokenCheckerAllowsWhenScriptAllowsFailOpen(t *testing.T) {
	for _, tc := range []struct {
		name     string
		doResult *redis.Cmd
	}{
		{
			name:     "missing active",
			doResult: redis.NewCmdResult(int64(1), nil),
		},
		{
			name:     "missing bloom filter",
			doResult: redis.NewCmdResult(int64(1), nil),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := &redisBloomTokenChecker{
				client: &stubRedisBloomClient{
					doResult: tc.doResult,
				},
				keyPrefix: "token:white",
			}

			allowed, err := checker.CheckToken(t.Context(), "raw-token")
			if err != nil {
				t.Fatalf("check token: %v", err)
			}
			if !allowed {
				t.Fatal("expected token to be allowed")
			}
		})
	}
}

func TestRedisBloomTokenCheckerReturnsRedisErrors(t *testing.T) {
	checkErr := errors.New("redis eval failed")
	checker := &redisBloomTokenChecker{
		client: &stubRedisBloomClient{
			doResult: redis.NewCmdResult(int64(0), checkErr),
		},
		keyPrefix: "token:white",
	}
	if _, err := checker.CheckToken(t.Context(), "raw-token"); err == nil {
		t.Fatal("expected eval error")
	}
}

type stubRedisBloomClient struct {
	doArgs   []any
	doResult *redis.Cmd
}

func (s *stubRedisBloomClient) Do(_ context.Context, args ...any) *redis.Cmd {
	s.doArgs = append([]any(nil), args...)
	if s.doResult != nil {
		return s.doResult
	}
	return redis.NewCmdResult(int64(0), redis.Nil)
}

func equalRedisArgs(got []any, want []any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
