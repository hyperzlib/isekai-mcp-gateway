// Package failure carries controlled failures and safe diagnostic metadata.
package failure

import (
	"context"
	"errors"
	"net"
	"strings"
)

const (
	Internal               = "INTERNAL_ERROR"
	Unavailable            = "SERVICE_UNAVAILABLE"
	Upstream               = "UPSTREAM_ERROR"
	Timeout                = "UPSTREAM_TIMEOUT"
	Canceled               = "REQUEST_CANCELED"
	InvalidToken           = "INVALID_TOKEN"
	AuthenticationRequired = "AUTHENTICATION_REQUIRED"
	InvalidMetadata        = "INVALID_CLIENT_METADATA"
	Capacity               = "REGISTRATION_CAPACITY_REACHED"
)

type Error struct {
	Code           string
	Message        string
	Op             string
	Retryable      bool
	Cause          error
	UpstreamStatus int
	Field          string
	Details        map[string]any
}

func (e *Error) Error() string   { return e.Message }
func (e *Error) Unwrap() error   { return e.Cause }
func (e *Error) Failure() *Error { return e }
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && e.Code == t.Code
}

var (
	ErrInvalidToken           = &Error{Code: InvalidToken, Message: "授权已失效，请重新授权。"}
	ErrAuthenticationRequired = &Error{Code: AuthenticationRequired, Message: "请先连接并完成授权。"}
)

func New(code, message, op string, cause error) *Error {
	return &Error{Code: code, Message: message, Op: op, Cause: cause}
}

// Wrap preserves a cause without exposing its text. Unknown failures are never
// automatically retryable. Only recognized dependency failures opt in.
func Wrap(op string, cause error) error {
	if cause == nil {
		return nil
	}
	// 已分类的错误沿用其 code，仅更新操作名与原始 cause。
	if f := Known(cause); f != nil {
		copy := *f
		copy.Op, copy.Cause = op, cause
		return &copy
	}
	if errors.Is(cause, context.Canceled) {
		return New(Canceled, "请求已取消。", op, cause)
	}
	// 仅上下文超时与网络超时归为可重试的 Timeout。
	if errors.Is(cause, context.DeadlineExceeded) {
		return &Error{Code: Timeout, Message: "服务请求超时，请稍后重试。", Op: op, Retryable: true, Cause: cause}
	}
	var n net.Error
	if errors.As(cause, &n) && n.Timeout() {
		return &Error{Code: Timeout, Message: "服务请求超时，请稍后重试。", Op: op, Retryable: true, Cause: cause}
	}

	return New(Internal, "服务内部错误，请通过错误编号联系管理员。", op, cause)
}

// Known 提取错误链中首个可识别的受控失败。
func Known(err error) *Error {
	var f interface{ Failure() *Error }
	if errors.As(err, &f) {
		return f.Failure()
	}
	return nil
}

func Describe(err error) *Error {
	if f := Known(err); f != nil {
		return f
	}
	if err == nil {
		return New(Internal, "服务内部错误。", "", nil)
	}
	return Wrap("", err).(*Error)
}

// Dependency 将未知依赖失败区分成不可用或上游错误。
func Dependency(op string, cause error) error {
	if cause == nil {
		return nil
	}
	f := Describe(Wrap(op, cause))
	if f.Code == Internal {
		// 网络操作错误或临时 DNS 失败视为可重试的不可用。
		var operation *net.OpError
		var dns *net.DNSError
		if errors.As(cause, &operation) || errors.As(cause, &dns) && dns.IsTemporary {
			return &Error{Code: Unavailable, Message: "服务暂不可用，请稍后重试。", Op: op, Retryable: true, Cause: cause}
		}
		return New(Upstream, "上游服务请求失败。", op, cause)
	}
	return f
}

// IsInfrastructure 判断错误是否为基础设施故障而非业务拒绝。
func IsInfrastructure(err error) bool {
	f := Describe(err)
	// wiki 操作产生的错误一律按业务处理。
	if f.Op == "wiki" {
		return false
	}
	switch f.Code {
	case InvalidToken, AuthenticationRequired, InvalidMetadata, Capacity,
		"INVALID_REQUEST", "INVALID_GRANT", "INVALID_CLIENT", "INVALID_SCOPE", "INVALID_REFRESH_TOKEN", "OAUTH_ERROR",
		"NO_SESSION", "INVALID_INTERACTION", "INVALID_LOGIN_TRANSACTION", "ACCOUNT_DISABLED", "LOGIN_FAILED",
		"UNKNOWN_HELP_TOPIC", "LOGIN_EXCHANGE_FAILED", "LOGIN_VERIFICATION_FAILED", "USER_INFORMATION_UNAVAILABLE", "WIKI_IDENTITY_UNAVAILABLE":
		return false
	default:
		// Bridge codes are mapped through a fixed whitelist by its integration.
		if strings.HasPrefix(f.Code, "isekai-servicebridge-") {
			return false
		}
		return true
	}
}

// HTTPStatus 将受控失败映射为对外 HTTP 状态码。
func HTTPStatus(err error) int {
	switch Describe(err).Code {
	case Unavailable, Capacity:
		return 503
	case Upstream:
		return 502
	case Timeout:
		return 504
	case InvalidToken, AuthenticationRequired:
		return 401
	case InvalidMetadata:
		return 400
	default:
		if !IsInfrastructure(err) {
			return 400
		}
		return 500
	}
}
