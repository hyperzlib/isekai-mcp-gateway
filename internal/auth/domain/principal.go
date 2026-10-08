package domain

import (
	"time"
)

type Principal struct {
	Issuer        string
	AccountID     string
	OAuthClientID string
	GrantID       string
	WikiUserID    int64
	Scopes        []string
	ExpiresAt     time.Time
}
