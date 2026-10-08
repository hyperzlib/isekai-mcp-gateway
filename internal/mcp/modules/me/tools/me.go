package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/auth/domain"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

type Input struct{}
type Output struct {
	User       domain.UserInfo     `json:"user"`
	ClientInfo *mcp.Implementation `json:"clientInfo"`
}

func Tool(a gatewaymcp.RequestUserReader) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "tool", Name: "wiki.me", Register: func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{Name: "wiki.me", Description: "获取当前 Wiki 用户信息和 MCP clientInfo。仅要求 mcp:access；clientInfo 由客户端自行声明，不代表已验证的 OAuth 客户端身份。", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(ctx context.Context, req *mcp.CallToolRequest, _ Input) (*mcp.CallToolResult, Output, error) {
				ctx = gatewaymcp.StartCall(ctx)
				user, err := a.MCPCurrentUser(ctx, req.Extra)
				if err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.me", err), Output{}, nil
				}
				out := Output{User: user, ClientInfo: req.ClientInfo()}
				text, err := json.Marshal(out)
				if err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.me", err), Output{}, nil
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, out, nil
			})
	}}
}
