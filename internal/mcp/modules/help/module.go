package help

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/help/resources"
	"isekai-mcp-gateway/internal/mcp/modules/help/tools"
	"isekai-mcp-gateway/internal/templates"
)

func Module(a gatewaymcp.RequestAuthorizer, renderer templates.TextRenderer) []gatewaymcp.Definition {
	s := New(renderer)
	return []gatewaymcp.Definition{tools.Tool(a, s), resources.IndexResource(a, s), resources.TopicResource(a, s)}
}
