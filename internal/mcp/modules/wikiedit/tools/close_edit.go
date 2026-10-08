package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func CloseEdit(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "close_edit", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) { return s.Close(ctx, c, i) })
}
