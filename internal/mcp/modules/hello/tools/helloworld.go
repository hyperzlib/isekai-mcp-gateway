package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

type Input struct {
	Name string `json:"name,omitempty" jsonschema:"Optional greeting name"`
}

type Output struct {
	Message string `json:"message"`
}

func Tool(a gatewaymcp.RequestAuthorizer) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "tool", Name: "wiki.helloworld", Register: func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{Name: "wiki.helloworld", Description: "Temporary tool for verifying authenticated MCP connections. Returns Hello, world! by default.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Output, error) {
				ctx = gatewaymcp.StartCall(ctx)
				if err := a.CheckMCPRequest(ctx, req.Extra); err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.helloworld", err), Output{}, nil
				}
				// 未提供 name 时回退到默认问候语。
				message := "Hello, world!"
				if input.Name != "" {
					message = fmt.Sprintf("Hello, %s!", input.Name)
				}
				out := Output{Message: message}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}}, out, nil
			})
	}}
}
