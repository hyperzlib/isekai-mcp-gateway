package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

type Input struct{}

type Output struct {
	Message   string `json:"message"`
	LoggedOut bool   `json:"logged_out"`
}

func Tool(a gatewaymcp.RequestLogout) gatewaymcp.Definition {
	// 退出会撤销授权，标记为破坏性操作以便客户端提示确认。
	destructive := true
	return gatewaymcp.Definition{Kind: "tool", Name: "wiki.logout", Register: func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{Name: "wiki.logout", Description: "退出当前 Gateway 授权：撤销当前 Grant 及其 access token、refresh token，后续请求需重新授权。保留客户端信任及浏览器 SSO 登录。", Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive}},
			func(ctx context.Context, req *mcp.CallToolRequest, _ Input) (*mcp.CallToolResult, Output, error) {
				ctx = gatewaymcp.StartCall(ctx)
				if err := a.LogoutMCPRequest(ctx, req.Extra); err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.logout", err), Output{}, nil
				}
				out := Output{Message: "已退出当前账号。如果需要重新登录，可能需要重启当前客户端或刷新MCP连接。", LoggedOut: true}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Message}}}, out, nil
			})
	}}
}
