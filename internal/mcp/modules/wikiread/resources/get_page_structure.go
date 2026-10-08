package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetPageStructure(s *wiki.Service) []gatewaymcp.Definition {
	return wikicommon.PageResources(s, "page-structure", "/structure", "get_page_structure", true)
}
