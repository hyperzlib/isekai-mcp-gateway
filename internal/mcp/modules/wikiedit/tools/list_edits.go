package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func ListEdits(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "list_edits", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) { return s.List(ctx, c, i) })
}
