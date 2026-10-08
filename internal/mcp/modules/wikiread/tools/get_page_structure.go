package tools

import (
	"context"

	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetPageStructure(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "get_page_structure", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Read(ctx, c, "get_page_structure", i)
	})
}
