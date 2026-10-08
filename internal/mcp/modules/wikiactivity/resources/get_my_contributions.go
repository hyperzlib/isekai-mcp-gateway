package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetMyContributions(s *wiki.Service) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{wikicommon.Resource(s, "my-contributions", "wiki://me/contributions", "application/json", "get_my_contributions")}
}
