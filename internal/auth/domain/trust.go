package domain

import (
	"time"
)

type ClientTrust struct {
	AccountID       string
	ClientID        string
	Fingerprint     string
	CreatedAt       time.Time
	LastConfirmedAt time.Time
}
