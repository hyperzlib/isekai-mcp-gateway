package domain

// UserInfo is the public Wiki identity; credentials and OIDC identifiers are excluded.
type UserInfo struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
