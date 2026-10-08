package hello

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/hello/tools"
)

func Module(a gatewaymcp.RequestAuthorizer) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{tools.Tool(a)}
}
