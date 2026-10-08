package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func BeginCreate(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "begin_create", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Begin(ctx, c, "begin_create", i)
	})
}
