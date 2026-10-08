package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func DeletePage(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "delete_page", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Mutation(ctx, c, "delete_page", i)
	})
}
