package wikicommon

import (
	"isekai-mcp-gateway/internal/wiki"
	"strings"
)

func Description(name string) string {
	descriptions := map[string]string{"update_edit": "Atomically apply ordered replace_content, replace_text, replace_lines, insert_lines, delete_lines or apply_patch operations. Requires base_version. Patch uses Begin/End Patch and @@ hunks without file headers; matches must be unique. Read wiki.help(editing) and wiki.help(patch).", "commit_edit": "Submit the draft with its saved public and pending baselines. Requires explicit ai_involvement. Confirm returned challenges in a separate call. After a network error read status and pull; do not retry blindly.", "pull_edit": "Synchronize current visible content and baselines. Every difference is placed in current/edited conflict blocks in the same draft. Resolve using update_edit before submitting.", "prepare_upload": "Prepare an immutable upload intent or recover using retry_upload_id only. Send raw bytes to the returned PUT URL, then call commit_upload.", "commit_upload": "Publish a staged upload. First warnings require a separate confirmation call. An unknown result cannot be published again.", "get_edit_content": "Read draft lines with display-only line numbers, or use mutually exclusive UTF-8 byte offset/limit. Fix expected_version across pages.", "search_edit_content": "Search the draft by literal text or Go linear-time regex, one line at a time. Returns bounded contexts and a version-bound cursor."}
	if d := descriptions[name]; d != "" {
		return d
	}
	return "Wiki " + strings.ReplaceAll(name, "_", " ") + ". Each call checks current authorization. Read wiki.help(index) for limits, confirmation and recovery instructions."
}
func Schema(name string) map[string]any {
	properties := map[string]any{}
	required := []string{}
	add := func(key, kind string, req bool) {
		v := map[string]any{"type": kind}
		properties[key] = v
		if req {
			required = append(required, key)
		}
	}
	fields := map[string]string{
		"resolve_page_url":       "url!",
		"list_subpages":          "page! limit cursor",
		"get_file_urls":          "titles!",
		"get_user_contributions": "user_id username limit cursor start end direction namespaces show tag",
		"get_my_contributions":   "limit cursor start end direction namespaces show tag",
		"get_recent_changes":     "user_id username limit cursor start end direction namespaces show tag types",
		"get_page_structure":     "page! revision_id limit cursor",
		"get_template_data":      "page! language",
		"get_page_dependencies":  "page! revision_id dependency_types limit cursor",
		"get_transclusions":      "page! namespaces limit cursor",
		"get_latest_page":        "page! format offset limit cursor", "get_baseline_page": "page! format offset limit cursor", "search": "query! limit cursor", "get_history": "page! limit cursor user start end", "get_revision": "revision_id! format offset limit", "compare_revisions": "from_revision! to_revision! offset limit", "get_backlinks": "page! limit cursor", "list_category": "category! limit cursor", "get_user": "user_id username",
		"begin_edit": "page! follow_redirect", "begin_create": "title!", "list_edits": "cursor limit", "get_edit_status": "edit_id!", "get_edit_content": "edit_id! start_line line_count include_line_numbers expected_version offset limit", "search_edit_content": "edit_id! pattern! regex case_sensitive context_lines max_matches cursor expected_version", "update_edit": "edit_id! base_version! operations!", "preview_edit": "edit_id! base_version!", "get_edit_diff": "edit_id! comparison context_lines offset limit expected_version expected_current_revision", "get_edit_conflict": "edit_id! cursor limit expected_version", "commit_edit": "edit_id! base_version! summary! ai_involvement! challenge_responses", "pull_edit": "edit_id! base_version!", "close_edit": "edit_id! base_version! challenge_responses", "move_page": "page! to_title! reason! challenge_responses", "delete_page": "page! reason! challenge_responses", "prepare_upload": "filename mime_type size_bytes sha256 comment overwrite retry_upload_id", "commit_upload": "upload_id! challenge_responses confirmed_warnings",
	}
	pageRead := wiki.IsPageRead(name)
	if pageRead {
		fields[name] = strings.Replace(fields[name], "page!", "title page_id", 1)
		if name == "get_latest_page" || name == "get_baseline_page" {
			fields[name] += " follow_redirects"
		}
	}

	// 用空格分隔的集合做 O(1) 类型判定；字段名用 "名 " 包裹避免子串误匹配。
	ints := " page_id offset limit user_id revision_id from_revision to_revision start_line line_count expected_version base_version context_lines max_matches expected_current_revision size_bytes "
	bools := " follow_redirect follow_redirects include_line_numbers regex case_sensitive overwrite "
	arrays := " titles namespaces show types dependency_types operations challenge_responses confirmed_warnings "
	for _, f := range strings.Fields(fields[name]) {
		// 后缀 "!" 表示必填字段。
		req := strings.HasSuffix(f, "!")
		k := strings.TrimSuffix(f, "!")
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
		if k == "page" {
			kind = "object"
		}
		add(k, kind, req)
		v := properties[k].(map[string]any)

		// 数值默认最小为 0；修订号类字段必须从 1 开始。
		if kind == "integer" {
			v["minimum"] = 0
		}
		if k == "base_version" || k == "revision_id" || k == "from_revision" || k == "to_revision" {
			v["minimum"] = 1
		}
		if kind == "array" {
			v["items"] = map[string]any{"type": "object"}
		}
		if k == "confirmed_warnings" {
			v["items"] = map[string]any{"type": "string"}
		}
	}
	// page 只能二选一：page_id 与 title 互斥。
	if name == "get_file_urls" {
		properties["titles"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 500, "items": map[string]any{"type": "string", "minLength": 1}}
	}
	if _, ok := properties["page"]; ok {
		properties["page"] = map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"page_id": map[string]any{"type": "integer", "minimum": 1}, "title": map[string]any{"type": "string", "minLength": 1}}, "oneOf": []any{map[string]any{"required": []string{"page_id"}, "not": map[string]any{"required": []string{"title"}}}, map[string]any{"required": []string{"title"}, "not": map[string]any{"required": []string{"page_id"}}}}}
	}

	// 每种编辑操作生成一个带 const type 的判别式变体，操作数上限 64。
	if _, ok := properties["operations"]; ok {
		variants := []any{}
		for _, spec := range []string{"replace_content content", "replace_text old_text new_text", "replace_lines start_line end_line content", "insert_lines line position content", "delete_lines start_line end_line", "apply_patch patch"} {
			a := strings.Fields(spec)
			ps := map[string]any{"type": map[string]any{"type": "string", "const": a[0]}}
			for _, k := range a[1:] {
				kind := "string"
				if k == "start_line" || k == "end_line" || k == "line" {
					kind = "integer"
				}
				ps[k] = map[string]any{"type": kind}
			}
			variants = append(variants, map[string]any{"type": "object", "additionalProperties": false, "properties": ps, "required": append([]string{"type"}, a[1:]...)})
		}
		properties["operations"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": map[string]any{"oneOf": variants}}
	}
	// 挑战应答必须显式 confirmed=true，防止误确认。
	if _, ok := properties["challenge_responses"]; ok {
		properties["challenge_responses"] = map[string]any{"type": "array", "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"challenge_id": map[string]any{"type": "string"}, "confirmed": map[string]any{"type": "boolean", "const": true}}, "required": []string{"challenge_id", "confirmed"}}}
	}
	if _, ok := properties["ai_involvement"]; ok {
		properties["ai_involvement"] = map[string]any{"type": "string", "enum": []string{"none", "ai_organized", "ai_assisted", "ai_generated"}}
	}
	result := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if name == "resolve_page_url" {
		properties["url"].(map[string]any)["minLength"] = 1
		properties["url"].(map[string]any)["description"] = "Allowed HTTP(S) page URL or site absolute path, such as /index.php?title=首页."
	}
	if name == "list_subpages" {
		properties["limit"].(map[string]any)["minimum"] = 1
		properties["limit"].(map[string]any)["maximum"] = 100
		properties["limit"].(map[string]any)["default"] = 20
	}
	if pageRead {
		result["oneOf"] = []any{map[string]any{"required": []string{"title"}, "not": map[string]any{"required": []string{"page_id"}}}, map[string]any{"required": []string{"page_id"}, "not": map[string]any{"required": []string{"title"}}}}
		properties["title"].(map[string]any)["minLength"] = 1
		properties["title"].(map[string]any)["description"] = "Page title including namespace. Preferred selector; mutually exclusive with page_id."
		properties["page_id"].(map[string]any)["minimum"] = 1
		if name == "get_page_structure" || name == "get_page_dependencies" {
			// 章节和依赖可直接绑定正式修订，三个定位字段只能出现一个。
			variants := []any{}
			for _, key := range []string{"title", "page_id", "revision_id"} {
				others := []any{}
				for _, other := range []string{"title", "page_id", "revision_id"} {
					if other != key {
						others = append(others, map[string]any{"required": []string{other}})
					}
				}
				variants = append(variants, map[string]any{"required": []string{key}, "not": map[string]any{"anyOf": others}})
			}
			result["oneOf"] = variants
		}
	}
	if len(required) > 0 {
		result["required"] = required
	}
	// get_user 必须且只能用 user_id 或 username 之一查询。
	if name == "get_user" || name == "get_user_contributions" {
		result["oneOf"] = []any{map[string]any{"required": []string{"user_id"}, "not": map[string]any{"required": []string{"username"}}}, map[string]any{"required": []string{"username"}, "not": map[string]any{"required": []string{"user_id"}}}}
		properties["user_id"].(map[string]any)["minimum"] = 1
		properties["username"].(map[string]any)["minLength"] = 1
	}
	if name == "get_recent_changes" {
		result["not"] = map[string]any{"required": []string{"user_id", "username"}}
		properties["user_id"].(map[string]any)["minimum"] = 1
		properties["username"].(map[string]any)["minLength"] = 1
	}
	if property, ok := properties["direction"].(map[string]any); ok {
		property["enum"] = []string{"older", "newer"}
		property["default"] = "older"
	}
	for _, key := range []string{"start", "end"} {
		if property, ok := properties[key].(map[string]any); ok && wiki.IsActivityRead(name) {
			property["description"] = "RFC3339 timestamp including timezone; range order must match direction."
		}
	}
	if property, ok := properties["namespaces"].(map[string]any); ok {
		property["items"] = map[string]any{"type": "integer", "minimum": 0}
		property["maxItems"] = 50
		property["uniqueItems"] = true
	}
	for key, values := range map[string]string{
		"types":            "edit new log categorize external",
		"dependency_types": "templates links images categories external_links",
		"show":             "minor !minor new !new top !top patrolled !patrolled autopatrolled !autopatrolled",
	} {
		if key == "show" && name == "get_recent_changes" {
			values = "minor !minor bot !bot anon !anon redirect !redirect patrolled !patrolled autopatrolled !autopatrolled"
		}
		if property, ok := properties[key].(map[string]any); ok {
			property["items"] = map[string]any{"type": "string", "enum": strings.Fields(values)}
			property["uniqueItems"] = true
			property["maxItems"] = len(strings.Fields(values))
		}
	}

	// prepare_upload 两种模式：全新上传需完整元数据，重试模式只能带 retry_upload_id。
	if name == "prepare_upload" {
		result["oneOf"] = []any{map[string]any{"required": []string{"filename", "mime_type", "size_bytes", "sha256", "comment"}, "not": map[string]any{"required": []string{"retry_upload_id"}}}, map[string]any{"required": []string{"retry_upload_id"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"filename"}}, map[string]any{"required": []string{"mime_type"}}, map[string]any{"required": []string{"size_bytes"}}, map[string]any{"required": []string{"sha256"}}, map[string]any{"required": []string{"comment"}}, map[string]any{"required": []string{"overwrite"}}}}}}
		properties["sha256"].(map[string]any)["pattern"] = "^[0-9a-f]{64}$"
		properties["size_bytes"].(map[string]any)["minimum"] = 1
	}

	// 行号模式与字节偏移模式互斥，禁止混用。
	if name == "get_edit_content" {
		result["not"] = map[string]any{"allOf": []any{map[string]any{"anyOf": []any{map[string]any{"required": []string{"offset"}}, map[string]any{"required": []string{"limit"}}}}, map[string]any{"anyOf": []any{map[string]any{"required": []string{"start_line"}}, map[string]any{"required": []string{"line_count"}}, map[string]any{"required": []string{"include_line_numbers"}}}}}}
	}

	// 输出格式枚举；get_revision 只支持源码格式。
	if _, ok := properties["format"]; ok {
		properties["format"].(map[string]any)["enum"] = []string{"source", "html"}
		if name == "get_revision" {
			properties["format"].(map[string]any)["enum"] = []string{"source"}
		}
	}
	if name == "get_edit_diff" {
		properties["comparison"].(map[string]any)["enum"] = []string{"origin_to_edited", "current_to_edited"}
	}

	// 统一限制分页类字段上限；行数上限放宽，而整篇读取类 limit 表示字节数不设上限。
	for _, key := range []string{"limit", "line_count", "max_matches"} {
		if v, ok := properties[key].(map[string]any); ok {
			v["maximum"] = 100
			if key == "line_count" {
				v["maximum"] = 2000
			}
			if key == "limit" && (name == "get_latest_page" || name == "get_baseline_page" || name == "get_revision" || name == "get_edit_content" || name == "get_edit_diff" || name == "compare_revisions") {
				delete(v, "maximum")
			}
		}
	}
	if v, ok := properties["context_lines"].(map[string]any); ok {
		v["maximum"] = 100
	}
	return result
}
