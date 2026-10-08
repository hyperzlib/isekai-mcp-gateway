package wikicommon

import (
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/wiki"
)

// 页面资源提供标题和 ID 两个入口；可绑定正式修订的读取另增加修订入口。
func PageResources(s *wiki.Service, name, suffix, operation string, revision bool) []gatewaymcp.Definition {
	defs := []gatewaymcp.Definition{
		Resource(s, name+"-by-title", "wiki://pages/by-title/{encoded_title}"+suffix, "application/json", operation),
		Resource(s, name+"-by-id", "wiki://pages/by-id/{page_id}"+suffix, "application/json", operation),
	}
	if revision {
		defs = append(defs, Resource(s, name+"-by-revision", "wiki://revisions/{revision_id}"+suffix, "application/json", operation))
	}
	return defs
}
