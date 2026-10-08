package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

type Input struct {
	Topic string `json:"topic,omitempty" jsonschema:"Help topic; defaults to index"`
}

func Tool(a gatewaymcp.RequestAuthorizer, service Reader) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "tool", Name: "wiki.help", Register: func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{Name: "wiki.help", Description: "Read Gateway help. Start with topic=index; available topics are connection, authorization, reading and editing.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Result, error) {
				ctx = gatewaymcp.StartCall(ctx)
				if err := a.CheckMCPRequest(ctx, req.Extra); err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.help", err), Result{}, nil
				}
				out, err := service.Read(input.Topic)
				if err != nil {
					return gatewaymcp.ToolError(ctx, req.Extra, "wiki.help", err), Result{}, nil
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out.Content}}}, out, nil
			})
	}}
}
