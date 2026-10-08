package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func Edits(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Resource(s, "edit-edits", "wiki://edits", "application/json", "list_edits")
}
