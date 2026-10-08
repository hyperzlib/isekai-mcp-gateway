package application

import "isekai-mcp-gateway/internal/failure"

type LoginResult struct{ SessionID, InteractionID string }

// FlowError carries a browser-safe explanation without exposing provider errors.
type FlowError struct {
	Message string
	Status  int
	Code    string
	Cause   error
}

func (e *FlowError) Error() string { return e.Message }

func (e *FlowError) Unwrap() error { return e.Cause }
func (e *FlowError) Failure() *failure.Error {
	// 未指定错误码时按状态码回退为登录失败或内部错误。
	code := e.Code
	if code == "" {
		code = "LOGIN_FAILED"
		if e.Status >= 500 {
			code = failure.Internal
		}
	}
	f := &failure.Error{Code: code, Message: e.Message, Cause: e.Cause}
	// 服务端错误保留底层失败的重试与上游状态信息。
	if cause := failure.Known(e.Cause); cause != nil && e.Status >= 500 {
		f.Code, f.Op, f.Retryable, f.UpstreamStatus = cause.Code, cause.Op, cause.Retryable, cause.UpstreamStatus
	}
	return f
}
