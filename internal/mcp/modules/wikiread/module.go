package wikiread

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/wikicommon"
	"isekai-mcp-gateway/internal/mcp/modules/wikiread/resources"
	"isekai-mcp-gateway/internal/mcp/modules/wikiread/tools"
	"isekai-mcp-gateway/internal/wiki"
)

func Module(s *wiki.Service) []gatewaymcp.Definition {
	defs := []gatewaymcp.Definition{
		tools.GetLatestPage(s), tools.GetBaselinePage(s), tools.Search(s),
		tools.GetHistory(s), tools.GetRevision(s), tools.CompareRevisions(s),
		tools.GetBacklinks(s), tools.ListCategory(s), tools.GetUser(s),
		tools.GetPageStructure(s), tools.GetTemplateData(s),
		tools.GetPageDependencies(s), tools.GetTransclusions(s),
		tools.GetFileURLs(s), tools.ListSubpages(s), tools.ResolvePageURL(s),
	}
	for _, spec := range []struct{ name, suffix, operation string }{
		{"page", "", "get_latest_page"},
		{"page-baseline", "/baseline", "get_baseline_page"},
		{"page-history", "/history", "get_history"},
		{"page-backlinks", "/backlinks", "get_backlinks"},
	} {
		defs = append(defs,
			wikicommon.Resource(s, spec.name+"-by-title", "wiki://pages/by-title/{encoded_title}"+spec.suffix, "application/json", spec.operation),
			wikicommon.Resource(s, spec.name+"-by-id", "wiki://pages/by-id/{page_id}"+spec.suffix, "application/json", spec.operation))
	}

	defs = append(defs, resources.GetPageStructure(s)...)
	defs = append(defs, resources.GetTemplateData(s)...)
	defs = append(defs, resources.GetPageDependencies(s)...)
	defs = append(defs, resources.GetTransclusions(s)...)
	return defs
}
