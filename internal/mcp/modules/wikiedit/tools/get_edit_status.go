package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func GetEditStatus(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "get_edit_status", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Get(ctx, c, "get_edit_status", i)
	})
}
