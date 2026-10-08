package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Diff(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-diff", "wiki://edit/{edit_id}/diff", "text/plain", "get_edit_diff")
}
