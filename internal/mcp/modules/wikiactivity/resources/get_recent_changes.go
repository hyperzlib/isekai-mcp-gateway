package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetRecentChanges(s *wiki.Service) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{wikicommon.Resource(s, "recent-changes", "wiki://recent-changes", "application/json", "get_recent_changes")}
}
