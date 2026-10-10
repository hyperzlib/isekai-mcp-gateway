package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func ResolvePageURL(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "resolve_page_url", true, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) {
		return s.Read(ctx, c, "resolve_page_url", i)
	})
}
