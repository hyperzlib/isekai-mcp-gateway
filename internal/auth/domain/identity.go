package domain

import "isekai-mcp-gateway/internal/failure"

type Identity struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	UserToken   string `json:"user_token"`
	// AvatarURL 是 IDP 侧头像地址，属非凭据信息，按明文 payload 存储。
	AvatarURL string `json:"avatar_url,omitempty"`
}

// IdentityError contains a controlled explanation, never upstream payloads.
type IdentityError struct {
	Code, Message  string
	Cause          error
	UpstreamStatus int
}

func (e *IdentityError) Error() string { return e.Message }

func (e *IdentityError) Unwrap() error { return e.Cause }
func (e *IdentityError) Failure() *failure.Error {
	// 缺少错误码时回退为身份不可用。
	code := e.Code
	if code == "" {
		code = "WIKI_IDENTITY_UNAVAILABLE"
	}
	return &failure.Error{Code: code, Message: e.Message, Cause: e.Cause, UpstreamStatus: e.UpstreamStatus}
}
