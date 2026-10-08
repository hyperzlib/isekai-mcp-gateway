package domain

import (
	"time"
)

type Grant struct {
	ID          string
	AccountID   string
	ClientID    string
	Resource    string
	Scopes      []string
	SessionID   string
	Fingerprint string
	AuthTime    time.Time
}
