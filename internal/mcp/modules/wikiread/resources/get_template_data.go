package resources

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetTemplateData(s *wiki.Service) []gatewaymcp.Definition {
	return wikicommon.PageResources(s, "template-data", "/template-data", "get_template_data", false)
}
