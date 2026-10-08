package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func PullEdit(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "pull_edit", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) { return s.Pull(ctx, c, i) })
}
