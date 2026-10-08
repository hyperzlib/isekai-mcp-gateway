package logout

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/logout/tools"
)

func Module(a gatewaymcp.RequestLogout) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{tools.Tool(a)}
}
