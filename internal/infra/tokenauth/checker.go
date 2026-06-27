package tokenauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

//nolint:gosec // This is a Redis Lua script, not a credential.
const tokenAuthCheckScript = `
local active = redis.call("GET", KEYS[1])
if not active or active == "" then
	return 1
end

local filter_key = KEYS[2] .. active
if redis.call("EXISTS", filter_key) == 0 then
	return 1
end

return redis.call("BF.EXISTS", filter_key, ARGV[1])
`

type RedisBloomConfig struct {
	Enabled   bool
	URL       string
	Password  string
	KeyPrefix string
}

type Checker interface {
	CheckToken(ctx context.Context, token string) (bool, error)
}

type NoopChecker struct{}

func (NoopChecker) CheckToken(context.Context, string) (bool, error) {
	return true, nil
}

type redisBloomClient interface {
	Do(ctx context.Context, args ...any) *redis.Cmd
}

type redisBloomTokenChecker struct {
	client    redisBloomClient
	keyPrefix string
}

func NewChecker(cfg RedisBloomConfig) (Checker, error) {
	if !cfg.Enabled {
		return NoopChecker{}, nil
	}

	rawURL := strings.TrimSpace(cfg.URL)
	if rawURL == "" {
		return nil, errors.New("token auth redis url is required")
	}
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse token auth redis url: %w", err)
	}
	if password := strings.TrimSpace(cfg.Password); password != "" {
		options.Password = password
	}

	keyPrefix := strings.Trim(strings.TrimSpace(cfg.KeyPrefix), ":")
	if keyPrefix == "" {
		return nil, errors.New("token auth redis key prefix is required")
	}

	return &redisBloomTokenChecker{
		client:    redis.NewClient(options),
		keyPrefix: keyPrefix,
	}, nil
}

func (c *redisBloomTokenChecker) CheckToken(ctx context.Context, token string) (bool, error) {
	if c == nil || c.client == nil {
		return true, nil
	}

	exists, err := c.client.Do(
		ctx,
		"EVAL",
		tokenAuthCheckScript,
		2,
		c.redisKey("active"),
		c.redisKey("bf")+":",
		tokenHash(token),
	).Int()
	if err != nil {
		return false, fmt.Errorf("check token auth bloom filter: %w", err)
	}
	return exists == 1, nil
}

func (c *redisBloomTokenChecker) redisKey(parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	segments = append(segments, c.keyPrefix)
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), ":")
		if part != "" {
			segments = append(segments, part)
		}
	}
	return strings.Join(segments, ":")
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
