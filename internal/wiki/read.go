package wiki

import (
	"context"
	"fmt"
	"isekai-mcp-gateway/internal/security"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func pages(v Object) ([]any, error) {
	q := object(v["query"])
	a, ok := q["pages"].([]any)

	// 契约要求上游只返回单个 page，否则视为响应异常。
	if !ok || len(a) != 1 {
		return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not return exactly one page.")
	}
	return a, nil
}
func (s *Service) snapshot(ctx context.Context, c *Call, p PageRef, follow bool) (Object, error) {
	return s.snapshotQuery(ctx, c, p, follow, nil)
}

func (s *Service) snapshotQuery(ctx context.Context, c *Call, p PageRef, follow bool, extra url.Values) (Object, error) {
	params, e := pageParams(p)
	if e != nil {
		return nil, e
	}
	q := url.Values(params)
	q.Set("action", "query")
	q.Set("prop", "info|revisions")
	q.Set("inprop", "url")
	q.Set("rvprop", "ids|timestamp|content|contentmodel")
	q.Set("rvslots", "main")
	q.Set("rvlimit", "1")

	// 标题规范化由上游完成，按需额外跟随重定向。
	if follow {
		q.Set("redirects", "1")
	}

	// Moderation 开启时改查审核状态，内容来源由上游决定。
	if c.Config.Static.Extensions.Moderation.Enabled {
		q.Set("prop", "info|moderationeditstate")
	}
	for key, values := range extra {
		q[key] = values
	}
	v, e := c.Client.Query(ctx, c.Credentials, q)
	if e != nil {
		return nil, e
	}
	a, e := pages(v)
	if e != nil {
		return nil, e
	}
	pg := object(a[0])
	id := number(pg["pageid"])

	// missing 表示页面不存在，invalid 表示标题非法，两者语义不同需区别处理。
	if _, missing := pg["missing"]; missing {
		id = 0
	}
	if _, invalid := pg["invalid"]; invalid {
		return nil, errCode("INVALID_REQUEST", "Wiki rejected the page title.")
	}

	// 负 pageid 归一为 0，表示无有效页面。
	if id < 0 {
		id = 0
	}
	out := Object{"page_id": nil, "title": pg["title"], "requested_title": p.Title, "baseline_exists": id > 0, "baseline_revision_id": nil, "revision_id": nil, "content_source": "empty", "content": "", "content_hash": nil}
	if p.Title == "" {
		delete(out, "requested_title")
	}
	out["url"] = ""
	if id > 0 {
		out["url"] = str(pg["fullurl"])
	}
	out["redirect_chain"] = []any{}
	if redirects, ok := object(v["query"])["redirects"].([]any); ok {
		out["redirect_chain"] = redirects
	}
	if id > 0 {
		out["page_id"] = id
	}
	if c.Config.Static.Extensions.Moderation.Enabled {
		st, ok := pg["moderationeditstate"].(map[string]any)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted moderation state.")
		}
		source := str(st["content_source"])

		// content_source 必须是已知三态之一。
		if source != "baseline" && source != "pending" && source != "empty" {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid moderation state.")
		}
		text, ok := st["content"].(string)
		if !ok {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted page content.")
		}

		// 将上游审核字段平铺到返回结果。
		for _, k := range []string{"page_id", "title", "content_source", "content", "content_hash", "temp_revision_id", "chain_version"} {
			out[k] = st[k]
		}
		out["baseline_revision_id"] = st["public_revision_id"]
		out["baseline_exists"] = number(st["public_revision_id"]) > 0
		out["content"] = text

		// 校验上游声明的哈希与内容一致，防止快照被篡改。
		if str(st["content_hash"]) != security.Hash(text) {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki snapshot hash does not match its content.")
		}
		if source == "baseline" {
			out["revision_id"] = st["public_revision_id"]
		}

		// pending 来源必须带有效临时修订 ID，否则无法定位快照。
		if source == "pending" && number(st["temp_revision_id"]) <= 0 {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Pending snapshot ID is missing.")
		}
	} else if id > 0 {
		rs, _ := pg["revisions"].([]any)
		if len(rs) != 1 {
			return nil, errCode("PERMISSION_DENIED", "Page revision is unavailable.")
		}
		r := object(rs[0])
		slot := object(object(r["slots"])["main"])
		text, ok := slot["content"].(string)
		if !ok {
			return nil, errCode("PERMISSION_DENIED", "Page content is unavailable.")
		}

		// 仅支持 wikitext，其他内容模型无法安全编辑。
		if m := str(slot["contentmodel"]); m != "" && m != "wikitext" {
			return nil, errCode("CONTENT_MODEL_UNSUPPORTED", "Only wikitext editing is supported.")
		}
		out["content"] = text
		out["content_source"] = "baseline"
		out["baseline_revision_id"] = r["revid"]
		out["revision_id"] = r["revid"]
		out["content_hash"] = security.Hash(text)
	}

	// 超出编辑上限的内容直接拒绝，避免后续写入被上游截断。
	if int64(len(str(out["content"]))) > c.Config.App.Editing.MaxContent {
		return nil, errCode("CONTENT_TOO_LARGE", "Page exceeds the content limit.")
	}
	if extra.Get("intestactions") != "" {
		out["actions"] = pg["actions"]
	}
	if strings.Contains(q.Get("prop"), "isekaiuseredited") && id > 0 {
		edited, err := pageContribution(pg)
		if err != nil {
			return nil, err
		}
		out["current_user"] = Object{"has_edited": edited}
	}
	return out, nil
}
func (s *Service) contribution(ctx context.Context, c *Call, id int64) (bool, error) {
	v, e := c.Client.Query(ctx, c.Credentials, url.Values{"action": {"query"}, "prop": {"isekaiuseredited"}, "pageids": {fmt.Sprint(id)}})
	if e != nil {
		return false, errCode("CONTRIBUTION_UNAVAILABLE", "Page contribution could not be checked.")
	}
	a, e := pages(v)
	if e != nil {
		return false, errCode("CONTRIBUTION_UNAVAILABLE", "Page contribution could not be checked.")
	}

	return pageContribution(object(a[0]))
}

func pageContribution(pg Object) (bool, error) {
	// has_edited 只接受 0/1，其他值视为上游返回异常。
	n, ok := object(pg["isekaiuseredited"])["has_edited"].(float64)
	if !ok || (n != 0 && n != 1) {
		return false, errCode("CONTRIBUTION_UNAVAILABLE", "Wiki returned invalid contribution information.")
	}
	return n == 1, nil
}

func (s *Service) revision(ctx context.Context, c *Call, id int64) (Object, error) {
	return s.revisionQuery(ctx, c, id, false)
}
func (s *Service) revisionQuery(ctx context.Context, c *Call, id int64, contribution bool) (Object, error) {
	if id <= 0 {
		return nil, errCode("INVALID_REQUEST", "revision_id must be positive.")
	}
	q := url.Values{"action": {"query"}, "revids": {fmt.Sprint(id)}, "prop": {"revisions"}, "rvprop": {"ids|timestamp|user|comment|content|contentmodel"}, "rvslots": {"main"}}
	if contribution {
		q.Set("prop", "revisions|isekaiuseredited")
	}
	v, e := c.Client.Query(ctx, c.Credentials, q)
	if e != nil {
		return nil, e
	}
	a, e := pages(v)
	if e != nil {
		return nil, e
	}
	pg := object(a[0])
	rs, _ := pg["revisions"].([]any)
	if len(rs) != 1 {
		return nil, errCode("NOT_FOUND", "Revision was not found.")
	}
	r := object(rs[0])
	text, ok := object(object(r["slots"])["main"])["content"].(string)
	if !ok {
		return nil, errCode("PERMISSION_DENIED", "Revision content is unavailable.")
	}
	out := Object{"page_id": pg["pageid"], "title": pg["title"], "revision_id": r["revid"], "timestamp": r["timestamp"], "user": r["user"], "comment": r["comment"], "content": text, "content_hash": security.Hash(text)}
	if contribution {
		edited, err := pageContribution(pg)
		if err != nil {
			return nil, err
		}
		out["current_user"] = Object{"has_edited": edited}
	}
	return out, nil
}
func fragment(text string, offset, limit int, max int64) (Object, error) {
	// offset 必须落在 UTF-8 字符起始字节，保证切片不会从字符中间开始。
	if offset < 0 || offset > len(text) || offset < len(text) && !utf8.RuneStart(text[offset]) {
		return nil, errCode("INVALID_REQUEST", "Offset must be a UTF-8 byte boundary.")
	}

	// limit 为 0 表示使用配置的最大字节数。
	if limit == 0 {
		limit = int(max)
	}
	if limit < 1 || int64(limit) > max {
		return nil, errCode("INVALID_REQUEST", "Invalid byte limit.")
	}
	end := offset + limit
	if end > len(text) {
		end = len(text)
	}

	// end 回退到字符边界，避免截断多字节字符。
	for end < len(text) && end > offset && !utf8.RuneStart(text[end]) {
		end--
	}

	// 剩余空间不足以容纳一个完整字符时拒绝，而不是返回空串。
	if end == offset && offset < len(text) {
		return nil, errCode("INVALID_REQUEST", "Limit is too small for a UTF-8 character.")
	}
	var next any

	// 仅当仍有剩余内容时返回 next_offset。
	if end < len(text) {
		next = end
	}
	return Object{"content": text[offset:end], "offset": offset, "next_offset": next, "truncated": end < len(text), "total_bytes": len(text)}, nil
}
func (s *Service) Read(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	if e := require(c, "wiki:read"); e != nil {
		return nil, e
	}
	if name == "resolve_page_url" {
		out, err := s.resolvePageURL(ctx, c, i)
		return boundedRead(c, out, err)
	}
	// 新增读取复用统一授权和返回大小限制，业务逻辑按活动、解析和模板分开。
	if IsActivityRead(name) {
		out, err := s.readActivity(ctx, c, name, i)
		return boundedRead(c, out, err)
	}
	if parsedPageRead(name) {
		out, err := s.readParsedPage(ctx, c, name, i)
		return boundedRead(c, out, err)
	}
	if name == "get_template_data" {
		out, err := s.readTemplateData(ctx, c, i)
		return boundedRead(c, out, err)
	}
	if name == "get_file_urls" {
		out, err := s.readFileURLs(ctx, c, i.Titles)
		return boundedRead(c, out, err)
	}
	if name == "get_transclusions" {
		out, err := s.readTransclusions(ctx, c, i)
		return boundedRead(c, out, err)
	}
	if name == "list_subpages" {
		out, err := s.readSubpages(ctx, c, i)
		return boundedRead(c, out, err)
	}
	if IsPageRead(name) {
		if i.Page.PageID != 0 || i.Page.Title != "" {
			return nil, errCode("INVALID_REQUEST", "Use top-level title or page_id for page reads.")
		}
		i.Page = PageRef{PageID: i.PageID, Title: i.Title}
		if _, e := pageParams(i.Page); e != nil {
			return nil, e
		}
	}
	// 页面与修订类工具单独处理，其余操作走通用列表分页。
	switch name {
	case "get_latest_page", "get_baseline_page":
		out, err := s.readPage(ctx, c, name, i)
		return boundedRead(c, out, err)
	case "get_revision":
		v, e := s.revision(ctx, c, i.RevisionID)
		if e != nil {
			return nil, e
		}
		if i.Format != "" && i.Format != "source" {
			return nil, errCode("INVALID_REQUEST", "Revision format must be source.")
		}
		f, e := fragment(str(v["content"]), i.Offset, i.Limit, c.Config.App.Results.TextMax)
		if e != nil {
			return nil, e
		}
		for k, x := range f {
			v[k] = x
		}
		return v, nil
	case "compare_revisions":
		if i.FromRevision <= 0 || i.ToRevision <= 0 {
			return nil, errCode("INVALID_REQUEST", "Revision IDs must be positive.")
		}
		v, e := c.Client.Query(ctx, c.Credentials, url.Values{"action": {"compare"}, "fromrev": {fmt.Sprint(i.FromRevision)}, "torev": {fmt.Sprint(i.ToRevision)}, "prop": {"diff|ids|title"}})
		if e != nil {
			return nil, e
		}
		out := object(v["compare"])
		text := str(out["body"])

		// diff 正文经 fragment 分页，避免超大 diff 一次性返回。
		f, e := fragment(text, i.Offset, i.Limit, c.Config.App.Results.TextMax)
		if e != nil {
			return nil, e
		}
		delete(out, "body")
		out["diff"] = f["content"]
		delete(f, "content")
		for k, x := range f {
			out[k] = x
		}
		return out, nil
	case "get_user":
		// user_id 与 username 必须二选一，且 user_id 不能为负。
		if (i.UserID > 0) == (i.Username != "") || i.UserID < 0 {
			return nil, errCode("INVALID_REQUEST", "Supply exactly one user_id or username.")
		}
		q := url.Values{"action": {"query"}, "list": {"users"}, "usprop": {"groups|editcount|registration"}}
		if i.UserID > 0 {
			q.Set("ususerids", fmt.Sprint(i.UserID))
		} else {
			// 用户名含 | 会破坏上游多值分隔语义，直接拒绝。
			if strings.Contains(i.Username, "|") {
				return nil, errCode("INVALID_REQUEST", "Invalid username.")
			}
			q.Set("ususers", i.Username)
		}
		v, e := c.Client.Query(ctx, c.Credentials, q)
		if e != nil {
			return nil, e
		}
		return Object{"users": object(v["query"])["users"]}, nil
	}
	limit, e := positive(i.Limit, 20, 100)
	if e != nil {
		return nil, e
	}

	// 指纹绑定查询参数，防止游标被挪用到其他查询。
	filter := fingerprint(Object{"page": i.Page, "query": i.Query, "category": i.Category, "user": i.User, "start": i.Start, "end": i.End, "limit": limit})
	cur, e := readCursor(i.Cursor, name, c.owner(c.Client.Config.SiteID), filter)
	if e != nil {
		return nil, e
	}
	q := url.Values{"action": {"query"}}
	key := ""
	switch name {
	case "search":
		if strings.TrimSpace(i.Query) == "" {
			return nil, errCode("INVALID_REQUEST", "Search query is required.")
		}
		q.Set("list", "search")
		q.Set("srsearch", i.Query)
		q.Set("srlimit", strconv.Itoa(limit))
		key = "search"
	case "get_history":
		p, e := pageParams(i.Page)
		if e != nil {
			return nil, e
		}
		q = url.Values(p)
		q.Set("action", "query")
		q.Set("prop", "revisions")
		q.Set("rvprop", "ids|timestamp|user|comment|size|flags")
		q.Set("rvlimit", strconv.Itoa(limit))
		if i.User != "" {
			q.Set("rvuser", i.User)
		}
		if i.Start != "" {
			q.Set("rvstart", i.Start)
		}
		if i.End != "" {
			q.Set("rvend", i.End)
		}
		key = "pages"
	case "get_backlinks":
		p, e := pageParams(i.Page)
		if e != nil {
			return nil, e
		}
		q.Set("list", "backlinks")
		if i.Page.PageID > 0 {
			q.Set("blpageid", p["pageids"][0])
		} else {
			q.Set("bltitle", p["titles"][0])
		}
		q.Set("bllimit", strconv.Itoa(limit))
		key = "backlinks"
	case "list_category":
		if i.Category == "" || strings.Contains(i.Category, "|") {
			return nil, errCode("INVALID_REQUEST", "Category is required.")
		}
		q.Set("list", "categorymembers")
		category := i.Category

		// 未带命名空间前缀时默认补 Category:。
		if !strings.Contains(category, ":") {
			category = "Category:" + category
		}
		q.Set("cmtitle", category)
		q.Set("cmlimit", strconv.Itoa(limit))
		key = "categorymembers"
	default:
		return nil, errCode("INVALID_REQUEST", "Unknown Wiki read operation.")
	}
	// 只允许已知的继续参数透传给上游，防止游标注入任意请求参数。
	allowed := map[string]bool{"continue": true, "sroffset": true, "rvcontinue": true, "blcontinue": true, "cmcontinue": true}
	for k, v := range cur.Continue {
		if !allowed[k] {
			return nil, errCode("INVALID_CURSOR", "Cursor contains an unsupported continuation.")
		}
		q.Set(k, fmt.Sprint(v))
	}
	v, e := c.Client.Query(ctx, c.Credentials, q)
	if e != nil {
		return nil, e
	}
	var next any

	// 上游返回 continue 时签发新的签名游标。
	if co, ok := v["continue"].(map[string]any); ok {
		next = cursor(cursorData{Tool: name, Owner: c.owner(c.Client.Config.SiteID), Filter: filter, Continue: co})
	}
	return Object{"results": object(v["query"])[key], "next_cursor": next}, nil
}
