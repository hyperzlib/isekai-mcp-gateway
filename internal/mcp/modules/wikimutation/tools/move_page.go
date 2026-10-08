package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func MovePage(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "move_page", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Mutation(ctx, c, "move_page", i)
	})
}
