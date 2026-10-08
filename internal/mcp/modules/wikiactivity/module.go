package wikiactivity

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikiactivity/resources"
	"isekai-mcp-gateway/internal/mcp/modules/wikiactivity/tools"
	"isekai-mcp-gateway/internal/wiki"
)

func Module(s *wiki.Service) []gatewaymcp.Definition {
	defs := []gatewaymcp.Definition{
		tools.GetUserContributions(s),
		tools.GetMyContributions(s),
		tools.GetRecentChanges(s),
	}

	defs = append(defs, resources.GetUserContributions(s)...)
	defs = append(defs, resources.GetMyContributions(s)...)
	defs = append(defs, resources.GetRecentChanges(s)...)
	return defs
}
