package repository

import (
	"errors"
	"time"
)

var ErrAPITokenNotFound = errors.New("api token not found")

type APITokenGrant struct {
	APIKeyID      string
	UserID        string
	RelayTargetID string
}

type APITokenLookup struct {
	Token string
	At    time.Time
}
