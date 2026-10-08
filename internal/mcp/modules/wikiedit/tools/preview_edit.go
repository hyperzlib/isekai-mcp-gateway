package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func PreviewEdit(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "preview_edit", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Preview(ctx, c, i)
	})
}
