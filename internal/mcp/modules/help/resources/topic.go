package resources

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
)

func TopicResource(a gatewaymcp.RequestAuthorizer, service Reader) gatewaymcp.Definition {
	return gatewaymcp.Definition{Kind: "template", Name: "help-topic", Key: "wiki://help/{topic}", Register: func(s *mcp.Server) {
		s.AddResourceTemplate(&mcp.ResourceTemplate{Name: "help-topic", URITemplate: "wiki://help/{topic}", MIMEType: "text/markdown", Description: "Read wiki.help(index) for the currently available help topics, including editing, wikitext, page-creation, patch, confirmation, uploads and recovery instructions."},
			func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				ctx = gatewaymcp.StartCall(ctx)
				if err := a.CheckMCPRequest(ctx, req.Extra); err != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, "help.resource", err)
				}

				// 从 URI 中取出主题名，交由 Read 做白名单校验。
				out, err := service.Read(strings.TrimPrefix(req.Params.URI, "wiki://help/"))
				if err != nil {
					return nil, gatewaymcp.ResourceError(ctx, req.Extra, "help.resource", err)
				}
				return resourceResult(out), nil
			})
	}}
}
