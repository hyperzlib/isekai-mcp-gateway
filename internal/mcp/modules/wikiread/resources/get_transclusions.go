package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetTransclusions(s *wiki.Service) []gatewaymcp.Definition {
	return wikicommon.PageResources(s, "page-transclusions", "/transclusions", "get_transclusions", false)
}
