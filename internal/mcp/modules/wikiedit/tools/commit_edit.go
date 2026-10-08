package tools

import (
	"context"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/wiki"
)

func CommitEdit(s *wiki.Service) gatewaymcp.Definition {
	return wikicommon.Tool(s, "commit_edit", false, func(ctx context.Context, c *wiki.Call, i wiki.Input) (wiki.Object, error) { return s.Commit(ctx, c, i) })
}
