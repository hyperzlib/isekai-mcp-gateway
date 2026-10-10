package wikicommon

import (
	"isekai-mcp-gateway/internal/config"
	"strings"
)

// Known fields are described; API result metadata may add fields without hiding
// the stable Gateway envelope from clients.
func OutputSchema(name string) map[string]any {
	if name == "resolve_page_url" {
		return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"title", "page_id", "exists"}, "properties": map[string]any{
			"title":   map[string]any{"type": "string", "minLength": 1},
			"page_id": map[string]any{"type": "integer", "minimum": 0},
			"exists":  map[string]any{"type": "boolean"},
		}}
	}
	fields := map[string]string{
		"list_subpages":          "page_id title requested_title results next_cursor queried_at",
		"get_file_urls":          "file_urls",
		"get_user_contributions": "user_id username results next_cursor queried_at resources",
		"get_my_contributions":   "user_id results next_cursor queried_at resources",
		"get_recent_changes":     "user_id username results next_cursor queried_at resources",
		"get_page_structure":     "page_id title requested_title revision_id content_source results total next_cursor resources",
		"get_template_data":      "page_id title requested_title has_template_data data queried_at resources",
		"get_page_dependencies":  "page_id title requested_title revision_id content_source results total next_cursor resources",
		"get_transclusions":      "page_id requested_title results next_cursor queried_at resources",
		"get_latest_page":        "page_id title baseline_exists baseline_revision_id revision_id content_source content content_hash current_user temp_revision_id chain_version offset next_offset truncated total_bytes", "get_baseline_page": "page_id title baseline_exists baseline_revision_id revision_id content_source content content_hash current_user temp_revision_id chain_version offset next_offset truncated total_bytes",
		"get_revision": "page_id title revision_id timestamp user comment content content_hash offset next_offset truncated total_bytes", "search": "results next_cursor", "get_history": "results next_cursor", "get_backlinks": "results next_cursor", "list_category": "results next_cursor", "get_user": "users", "compare_revisions": "diff offset next_offset truncated total_bytes",
		"list_edits": "edits next_cursor", "get_edit_content": "edit_id url version content content_hash content_source start_line end_line total_lines returned_lines next_line offset next_offset truncated total_bytes", "search_edit_content": "edit_id version search_results contexts next_cursor truncated", "get_edit_diff": "edit_id version edited_version comparison current_revision_id diff content_hash offset next_offset truncated total_bytes", "get_edit_conflict": "edit_id version conflict_count conflict_markers_valid conflict_ranges next_cursor content_uri",
		"commit_edit": "edit_id url url_refresh_warning outcome changed revision_id content_source version title page_id base_revision_id moderation_base_revision_id mod_id temp_revision_id chain_version has_changes", "close_edit": "edit_id closed close_requested version", "prepare_upload": "upload_id state upload_url method headers expires_at staged_expires_at next_tool warnings", "commit_upload": "upload_id state filename imageinfo",
	}
	for _, name := range []string{"get_latest_page", "get_baseline_page"} {
		fields[name] += " url requested_title redirect_chain resources next_cursor file_urls"
	}
	// 编辑类工具共享同一套状态字段。
	status := "edit_id page_id title url kind state version has_changes close_requested expires_at base_revision_id moderation_base_revision_id content_source content_hash content_bytes total_lines conflict_count conflict_ranges conflict_markers_valid resources loading error changed operations_applied changed_ranges outcome"
	if name == "begin_edit" || name == "begin_create" || name == "get_edit_status" || name == "update_edit" || name == "pull_edit" {
		fields[name] = status
	}
	if name == "preview_edit" {
		fields[name] = "edit_id version html content_hash"
	}

	// 按空格分隔的字段集合推断 JSON 类型。
	props := map[string]any{}
	ints := " user_id total page_id baseline_revision_id revision_id temp_revision_id chain_version offset next_offset total_bytes version start_line end_line total_lines returned_lines next_line edited_version current_revision_id conflict_count base_revision_id moderation_base_revision_id mod_id content_bytes operations_applied "
	bools := " has_template_data baseline_exists truncated changed has_changes closed close_requested conflict_markers_valid "
	arrays := " redirect_chain results users edits search_results contexts conflict_ranges changed_ranges "
	objects := " url_refresh_warning file_urls data current_user headers imageinfo warnings resources loading error "
	for _, k := range strings.Fields(fields[name]) {
		// 未启用审核扩展时，隐藏审核相关字段，避免向客户端暴露未提供的契约。
		cfg := config.GetConfig()
		if cfg == nil || !cfg.Static.Extensions.Moderation.Enabled {
			if k == "moderation_base_revision_id" || k == "temp_revision_id" || k == "chain_version" || k == "mod_id" {
				continue
			}
		}

		kind := "string"
		if strings.Contains(ints, " "+k+" ") {
			kind = "integer"
		}
		if strings.Contains(bools, " "+k+" ") {
			kind = "boolean"
		}
		if strings.Contains(arrays, " "+k+" ") {
			kind = "array"
		}
		if strings.Contains(objects, " "+k+" ") {
			kind = "object"
		}
		// 每个字段允许 null，保持对缺失值的宽容。
		v := map[string]any{"type": kind}
		if k == "url" {
			v["description"] = "Page access URL from MediaWiki; empty until the page has a public page ID."
		}
		if k == "file_urls" {
			v["additionalProperties"] = map[string]any{"type": "string"}
			v["description"] = "For source reads, maps 文件: titles used by the complete snapshot to original file URLs, separately from content."
		}
		if kind == "array" {
			v["items"] = map[string]any{}
		}
		props[k] = map[string]any{"anyOf": []any{v, map[string]any{"type": "null"}}}
	}

	// additionalProperties=true 允许 API 追加字段而不破坏稳定信封。
	return map[string]any{"type": "object", "properties": props, "additionalProperties": true}
}
