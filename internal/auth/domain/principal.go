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
	// AuthKind 区分凭据来源："oauth"（经授权签发的令牌）或 "api_key"（sk- 密钥）。
	AuthKind string
}

// OAuth 与 API key 两类 principal 的标记值。
const (
	AuthKindOAuth  = "oauth"
	AuthKindAPIKey = "api_key"
)
