package wikiedit

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/mcp/modules/wikiedit/resources"
	"isekai-mcp-gateway/internal/mcp/modules/wikiedit/tools"
	"isekai-mcp-gateway/internal/wiki"
)

func Module(s *wiki.Service) []gatewaymcp.Definition {
	return []gatewaymcp.Definition{tools.BeginEdit(s), tools.BeginCreate(s), tools.ListEdits(s), tools.GetEditStatus(s), tools.GetEditContent(s), tools.SearchEditContent(s), tools.UpdateEdit(s), tools.PreviewEdit(s), tools.GetEditDiff(s), tools.GetEditConflict(s), tools.CommitEdit(s), tools.PullEdit(s), tools.CloseEdit(s), resources.Edits(s), resources.Status(s), resources.Content(s), resources.Diff(s), resources.Preview(s), resources.Conflict(s), wikicommon.DynamicList(s)}
}
