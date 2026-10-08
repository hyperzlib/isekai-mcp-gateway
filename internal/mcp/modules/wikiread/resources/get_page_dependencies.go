package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetPageDependencies(s *wiki.Service) []gatewaymcp.Definition {
	return wikicommon.PageResources(s, "page-dependencies", "/dependencies", "get_page_dependencies", true)
}
