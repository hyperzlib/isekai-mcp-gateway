package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetUserContributions(s *wiki.Service) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{
		wikicommon.Resource(s, "user-contributions-by-id", "wiki://users/by-id/{user_id}/contributions", "application/json", "get_user_contributions"),
		wikicommon.Resource(s, "user-contributions-by-name", "wiki://users/by-name/{encoded_username}/contributions", "application/json", "get_user_contributions"),
	}
}
