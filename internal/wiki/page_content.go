package wiki

import (
	"context"
	"fmt"
	"isekai-mcp-gateway/internal/security"
	"net/url"
)

func (s *Service) readPage(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	if _, x := pageParams(i.Page); x != nil {
		return nil, x
	}
	// format 默认 source，仅允许 source 与 html。
	format := i.Format
	if format == "" {
		format = "source"
	}
	if format != "source" && format != "html" {
		return nil, errCode("INVALID_REQUEST", "format must be source or html.")
	}

	// 指纹绑定页面、格式与 limit，避免游标跨请求复用。
	filter := fingerprint(Object{"page": i.Page, "format": format, "limit": i.Limit, "follow_redirects": i.FollowRedirects})
	owner := c.owner(c.Client.Config.SiteID)
	cur, x := readCursor(i.Cursor, name, owner, filter)
	if x != nil {
		return nil, x
	}
	scope := c.cacheScope(name, filter)
	generation := s.pageCache.begin(scope, i.Cursor == "")
	cacheID := str(cur.Continue["cache_id"])
	linksHash := str(cur.Continue["linked_pages_hash"])
	var cachedText string
	cacheHit := false
	var v Object
	if i.Cursor == "" {
		// 首次读取不允许带 offset，分页必须通过游标固定快照。
		if i.Offset > 0 {
			return nil, errCode("INVALID_REQUEST", "Page fragments require the previous next_cursor to fix the snapshot.")
		}
		prop := "info|revisions|isekaiuseredited"
		if c.Config.Static.Extensions.Moderation.Enabled {
			prop = "info|moderationeditstate|isekaiuseredited"
		}
		v, x = s.snapshotQuery(ctx, c, i.Page, i.FollowRedirects, url.Values{"prop": {prop}})
		if x != nil {
			return nil, x
		}

		// 空内容源表示页面不存在。
		if v["content_source"] == "empty" {
			return nil, errCode("NOT_FOUND", "Page was not found.")
		}

		// get_baseline_page 遇到 pending 快照时改用公开基线修订内容。
		if name == "get_baseline_page" && v["content_source"] == "pending" {
			if id := number(v["baseline_revision_id"]); id > 0 {
				r, x := s.revision(ctx, c, id)
				if x != nil {
					return nil, x
				}
				v["content"] = r["content"]
				v["content_hash"] = r["content_hash"]
				v["revision_id"] = id
			} else {
				v["content"] = nil
				v["content_hash"] = nil
				v["revision_id"] = nil
			}
		}
	} else {
		// offset 必须与游标记录一致。
		if i.Offset != 0 && i.Offset != cur.Offset {
			return nil, errCode("INVALID_CURSOR", "Offset does not match the snapshot cursor.")
		}
		i.Offset = cur.Offset
		v = cur.Continue
		if v == nil {
			return nil, errCode("INVALID_CURSOR", "Snapshot cursor is invalid.")
		}

		if cached := s.pageCache.get(cacheID, scope); cached != nil {
			v = object(cached["snapshot"])
			cachedText = str(cached["html"])
			user, err := s.checkCachedRead(ctx, c, number(v["page_id"]), str(v["title"]), number(v["revision_id"]), true)
			if err != nil {
				return nil, err
			}
			if user != nil {
				v["current_user"] = user
			}
			cacheHit = true
		}
		if !cacheHit {
			// 基线快照按修订 ID 重新拉取内容。
			if id := number(v["revision_id"]); id > 0 {
				r, x := s.revisionQuery(ctx, c, id, true)
				if x != nil {
					return nil, x
				}
				v["content"] = r["content"]
				v["current_user"] = r["current_user"]
			} else if id := number(v["temp_revision_id"]); id > 0 && name == "get_latest_page" {
				// 待审快照访问依赖 Moderation 扩展。
				if !c.Config.Static.Extensions.Moderation.Enabled {
					return nil, errCode("FEATURE_UNAVAILABLE", "Pending snapshot access requires Moderation.")
				}
				p := PageRef{PageID: number(v["page_id"])}
				if p.PageID == 0 {
					p.Title = str(v["title"])
				}
				params, x := pageParams(p)
				if x != nil {
					return nil, x
				}
				q := url.Values(params)
				q.Set("action", "query")
				q.Set("prop", "moderationrevision")

				// 通过临时修订 ID 查询对应的审核快照。
				q.Set("mrtempid", fmt.Sprint(id))
				result, x := c.Client.Query(ctx, c.Credentials, q)
				if x != nil {
					return nil, x
				}
				a, x := pages(result)
				if x != nil {
					return nil, x
				}
				snap := object(object(a[0])["moderationrevision"])

				// 上游返回的临时修订 ID 必须与请求一致。
				if number(snap["temp_revision_id"]) != id {
					return nil, errCode("SNAPSHOT_UNAVAILABLE", "The selected pending snapshot is unavailable.")
				}
				text, ok := snap["content"].(string)
				if !ok {
					return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted snapshot content.")
				}
				v["content"] = text
			} else {
				v["content"] = nil
			}

		}
		// 内容哈希变化说明快照已被修改。
		if v["content"] != nil && security.Hash(str(v["content"])) != str(v["content_hash"]) {
			return nil, errCode("SNAPSHOT_UNAVAILABLE", "Selected snapshot content changed.")
		}
	}

	// Keep large link lists in the snapshot cache, with only their hash in cursors.
	delete(v, "linked_pages_hash")
	if format == "source" && (i.Cursor == "" || !cacheHit || v["linked_pages"] == nil) {
		files, links, err := s.pageSourceMetadata(ctx, c, v, i.Cursor == "")
		x = err
		if x != nil {
			return nil, x
		}
		hash := fingerprint(links)
		if linksHash != "" && linksHash != hash {
			return nil, errCode("SNAPSHOT_UNAVAILABLE", "Parsed page links changed. Restart the paginated read.")
		}
		linksHash = hash
		v["linked_pages"] = links
		if i.Cursor == "" {
			v["file_urls"] = files
		}
	}

	// 附带当前用户是否编辑过该页面。
	if id := number(v["page_id"]); id > 0 && v["current_user"] == nil {
		edited, x := s.contribution(ctx, c, id)
		if x != nil {
			return nil, x
		}
		v["current_user"] = Object{"has_edited": edited}
	}
	v["next_cursor"] = nil
	suffix := ""
	if name == "get_baseline_page" {
		suffix = "/baseline"
	}
	v["resources"] = PageResourceLinks(str(v["title"]), number(v["page_id"]), suffix)
	if v["content"] == nil {
		return v, nil
	}
	text := str(v["content"])
	if cacheHit && format == "html" {
		text = cachedText
	}
	if format == "html" && !cacheHit {
		q := url.Values{"action": {"parse"}, "prop": {"text"}}

		// 有修订 ID 时按 oldid 解析，否则按草稿文本预览。
		if id := number(v["revision_id"]); id > 0 {
			q.Set("oldid", fmt.Sprint(id))
		} else {
			q.Set("text", text)
			q.Set("title", str(v["title"]))
			q.Set("contentmodel", "wikitext")
		}
		result, x := c.Client.Query(ctx, c.Credentials, q)
		if x != nil {
			return nil, x
		}
		html, ok := object(result["parse"])["text"].(string)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted parsed HTML.")
		}
		text = html

		// 已分页读取时要求渲染结果稳定，否则让调用方重新开始。
		if previous := str(v["render_hash"]); previous != "" && previous != security.Hash(html) {
			return nil, errCode("SNAPSHOT_UNAVAILABLE", "Rendered HTML changed. Restart the paginated read.")
		}
		v["render_hash"] = security.Hash(html)
	}

	if !cacheHit {
		html := ""
		if format == "html" {
			html = text
		}
		cacheID = s.pageCache.put(c, scope, generation, number(v["page_id"]), str(v["title"]), Object{"snapshot": v, "html": html})
	}
	// 除内容与游标外的字段作为快照元数据存入游标。
	metadata := Object{}
	for k, value := range v {
		if k != "content" && k != "current_user" && k != "next_cursor" && k != "linked_pages" {
			metadata[k] = value
		}
	}
	metadata["cache_id"] = cacheID
	if format == "source" {
		metadata["linked_pages_hash"] = linksHash
	}
	delete(v, "cache_id")
	fragment, x := fragment(text, i.Offset, i.Limit, c.Config.App.Results.TextMax)
	if x != nil {
		return nil, x
	}
	for k, value := range fragment {
		v[k] = value
	}

	// 存在剩余内容时签发携带元数据的新游标。
	if next := number(fragment["next_offset"]); next > 0 {
		v["next_cursor"] = cursor(cursorData{Tool: name, Owner: owner, Filter: filter, Offset: int(next), Continue: metadata})
	}
	return v, nil
}
