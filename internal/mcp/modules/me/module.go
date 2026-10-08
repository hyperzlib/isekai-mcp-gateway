package me

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/me/tools"
)

func Module(a gatewaymcp.RequestUserReader) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{tools.Tool(a)}
}
