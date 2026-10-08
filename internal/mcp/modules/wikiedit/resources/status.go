package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Status(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-status", "wiki://edit/{edit_id}", "application/json", "get_edit_status")
}
