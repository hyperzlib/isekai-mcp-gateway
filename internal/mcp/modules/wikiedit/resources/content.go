package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Content(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-content", "wiki://edit/{edit_id}/content", "text/plain", "get_edit_content")
}
