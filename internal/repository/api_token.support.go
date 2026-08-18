package repository

import (
	"errors"
	"time"
)

var ErrAPITokenNotFound = errors.New("api token not found")

type APITokenGrant struct {
	APIKeyID      int64
	UserID        int64
	BindingID     string
	VendorRouteID string
}

type APITokenLookup struct {
	Token string
	At    time.Time
}
