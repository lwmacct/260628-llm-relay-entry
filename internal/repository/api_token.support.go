package repository

import (
	"errors"
	"time"
)

var ErrAPITokenNotFound = errors.New("api token not found")

type APITokenGrant struct {
	APIKeyID           int64
	UserID             int64
	BindingID          string
	VendorCredentialID string
}

type APITokenDigest struct {
	DigestKeyID string
	TokenDigest string
	At          time.Time
}
