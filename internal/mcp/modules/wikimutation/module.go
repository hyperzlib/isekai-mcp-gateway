package wikimutation

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikimutation/tools"
	"isekai-mcp-gateway/internal/wiki"
)

func Module(s *wiki.Service) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{tools.MovePage(s), tools.DeletePage(s), tools.PrepareUpload(s), tools.CommitUpload(s)}
}
