package gatewaymcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/failure"
)

type toolFailure struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	Retryable bool           `json:"retryable"`
	RequestID string         `json:"request_id"`
}

// StartCall creates an invocation identity independent of transport lifetime.
func StartCall(ctx context.Context) context.Context { return failure.NewRequest(ctx) }

func ToolError(ctx context.Context, extra *mcp.RequestExtra, tool string, err error) *mcp.CallToolResult {
	f := failure.Describe(err)

	// 仅透传格式合法的上游请求 ID 用于关联日志，非法则不采用。
	transportID := ""
	if extra != nil {
		candidate := extra.Header.Get("X-Request-ID")
		if failure.ValidRequestID(candidate) {
			transportID = candidate
		}
	}
	failure.Log(ctx, tool, err, "tool", tool, "transport_request_id", transportID)

	// 授权类底层错误统一降级为 INVALID_TOKEN，避免泄露内部状态细节。
	message := f.Message
	if f.Code == "INVALID_GRANT" || f.Code == "INVALID_CLIENT" || f.Code == "NO_SESSION" {
		f = failure.ErrInvalidToken
		message = f.Message
	}

	body, _ := json.Marshal(struct {
		Error toolFailure `json:"error"`
	}{toolFailure{
		Code: f.Code, Message: message, Details: safeDetails(f.Details), Retryable: f.Retryable, RequestID: failure.RequestID(ctx),
	}})
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}
}
func safeDetails(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

// ResourceError returns a protocol error with controlled business metadata.
func ResourceError(ctx context.Context, extra *mcp.RequestExtra, resource string, err error) error {
	result := ToolError(ctx, extra, resource, err)
	text := result.Content[0].(*mcp.TextContent).Text

	// 业务错误使用自定义码；基础设施故障映射为标准内部错误码，便于客户端区分。
	code := int64(-32000)
	if failure.IsInfrastructure(err) {
		code = jsonrpc.CodeInternalError
	}
	return &jsonrpc.Error{Code: code, Message: failure.Describe(err).Message, Data: json.RawMessage(text)}
}
