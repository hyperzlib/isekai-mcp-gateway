package resources

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

func IndexResource(a gatewaymcp.RequestAuthorizer, service Reader) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "resource", Name: "help-index", Key: "wiki://help/index", Register: func(s *mcp.Server) {
		s.AddResource(&mcp.Resource{Name: "help-index", URI: "wiki://help/index", MIMEType: "text/markdown", Description: "Available Gateway tools and help topics"},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				ctx = gatewaymcp.StartCall(ctx)
				if err := a.CheckMCPRequest(ctx, req.Extra); err != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, "help.resource", err)
				}
				out, err := service.Read("index")
				if err != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, "help.resource", err)
				}
				return resourceResult(out), nil
			})
	}}
}

func resourceResult(out Result) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: out.URI, MIMEType: "text/markdown", Text: out.Content}}}
}
