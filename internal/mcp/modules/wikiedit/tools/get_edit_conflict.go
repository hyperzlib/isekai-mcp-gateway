package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetEditConflict(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "get_edit_conflict", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Get(ctx, c, "get_edit_conflict", i)
	})
}
