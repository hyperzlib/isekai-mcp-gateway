package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Conflict(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-conflict", "wiki://edit/{edit_id}/conflict", "application/json", "get_edit_conflict")
}
