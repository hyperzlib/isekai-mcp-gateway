package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Preview(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-preview", "wiki://edit/{edit_id}/preview", "text/html", "preview_edit")
}
