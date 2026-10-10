package failure

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

const responseLogLimit = 8192

// UpstreamTrace 描述一次上游 HTTP 请求往返，请求与响应均已脱敏。
type UpstreamTrace struct {
	Method, URL    string
	RequestHeaders string // 顺序稳定的请求头，密钥类头的值已替换为掩码
	RequestBody    string // 已脱敏的请求体
	Status         int    // 上游 HTTP 状态码；未收到响应时为 0
	ResponseBody   string // 已截断的上游响应体
}

// RedactFormValues 深拷贝表单并将密钥字段的值替换为掩码。
func RedactFormValues(form url.Values, secretKeys ...string) url.Values {
	out := make(url.Values, len(form))
	for k, v := range form {
		out[k] = append([]string(nil), v...)
	}
	for _, k := range secretKeys {
		for i := range out[k] {
			out[k][i] = "[REDACTED]"
		}
	}
	return out
}

// FormatRequestHeaders 以稳定顺序返回请求头，密钥类头的值替换为掩码。
func FormatRequestHeaders(r *http.Request, secretKeys ...string) string {
	redacted := map[string]bool{}
	for _, k := range secretKeys {
		redacted[strings.ToLower(k)] = true
	}
	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		value := r.Header.Get(k)
		if redacted[strings.ToLower(k)] {
			value = "[REDACTED]"
		}
		lines = append(lines, k+": "+value)
	}
	return strings.Join(lines, "\n")
}

// ClipResponseBody 将响应体截断为固定上限并改写无效字节，避免日志体积失控。
func ClipResponseBody(b []byte) string {
	text := strings.ToValidUTF8(string(b), string(utf8.RuneError))
	if len(text) > responseLogLimit {
		return text[:responseLogLimit] + "…(truncated)"
	}
	return text
}

// TraceUpstream 在上游请求往返失败时以 warn 级别输出脱敏诊断。
// request_id 与错误页展示的错误编号一致，便于按编号关联排查。
func TraceUpstream(ctx context.Context, op string, t UpstreamTrace, err error) {
	fields := []any{
		"request_id", RequestID(ctx),
		"operation", op,
		"upstream_method", t.Method,
		"upstream_url", t.URL,
	}
	if err != nil {
		fields = append(fields, "error_code", Describe(err).Code)
	}
	if t.RequestHeaders != "" {
		fields = append(fields, "upstream_request_headers", t.RequestHeaders)
	}
	if t.RequestBody != "" {
		fields = append(fields, "upstream_request_body", t.RequestBody)
	}
	if t.Status != 0 {
		fields = append(fields, "upstream_response_status", t.Status)
	}
	if t.ResponseBody != "" {
		fields = append(fields, "upstream_response_body", t.ResponseBody)
	}
	slog.WarnContext(ctx, "upstream request exchanged", fields...)
}
