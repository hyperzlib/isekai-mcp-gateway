package domain

import (
	"isekai-mcp-gateway/internal/failure"
)

type OAuthError struct {
	Code, Description string
	Cause             error
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}
func (e *OAuthError) WithDescription(value string) *OAuthError { e.Description = value; return e }
func ErrInvalidRequest() *OAuthError                           { return &OAuthError{Code: "invalid_request"} }
func ErrInvalidGrant() *OAuthError                             { return &OAuthError{Code: "invalid_grant"} }
func ErrInvalidClient() *OAuthError                            { return &OAuthError{Code: "invalid_client"} }
func ErrInvalidScope() *OAuthError                             { return &OAuthError{Code: "invalid_scope"} }
func ErrServerError() *OAuthError                              { return &OAuthError{Code: "server_error"} }

var ErrInvalidRefreshToken = failure.New("INVALID_REFRESH_TOKEN", "invalid refresh token", "auth.refresh", nil)

func (e *OAuthError) Unwrap() error { return e.Cause }
func (e *OAuthError) Failure() *failure.Error {
	// 将 OAuth 错误码映射为内部失败码，server_error 归为内部错误。
	code := "OAUTH_ERROR"
	switch e.Code {
	case "invalid_request":
		code = "INVALID_REQUEST"
	case "invalid_grant":
		code = "INVALID_GRANT"
	case "invalid_client":
		code = "INVALID_CLIENT"
	case "invalid_scope":
		code = "INVALID_SCOPE"
	case "server_error":
		code = failure.Internal
	}
	return &failure.Error{Code: code, Message: e.Error(), Cause: e.Cause}
}
