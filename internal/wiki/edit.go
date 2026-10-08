package wiki

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"isekai-mcp-gateway/internal/security"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func (s *Service) checked(e Edit, c *Call, version int64, read bool) error {
	// 所有者不匹配时统一返回 NOT_FOUND，避免泄露他人会话存在性
	if e.Owner != c.owner(e.SiteID) {
		return errCode("NOT_FOUND", "Edit session was not found.")
	}

	// 过期会话不可用
	if !e.ExpiresAt.After(time.Now()) {
		return errCode("EDIT_SESSION_EXPIRED", "Edit session has expired.")
	}
	if e.EndedAt != nil {
		return errCode("EDIT_SESSION_CLOSED", "Edit session has ended.")
	}

	// 乐观锁：传入的版本必须与当前草稿版本一致
	if version > 0 && version != e.Version {
		return detail("EDIT_SESSION_VERSION_MISMATCH", "Read the current draft and retry with its version.", Object{"expected_version": version, "actual_version": e.Version})
	}

	// 非只读操作还需校验对应编辑权限
	if !read {
		if x := require(c, "wiki:"+e.Kind); x != nil {
			return x
		}
	}
	return nil
}
func endEdit(e *Edit) {
	// 若仍处于 loading，先回退到占用前状态，避免结束后遗留临时状态
	if e.State == "loading" {
		e.State = e.PreviousState
		if e.State == "" {
			e.State = "ready"
		}
	}
	now := time.Now().UTC()
	e.EndedAt = &now
	e.EndReason = "closed"
	e.Content = ""
	e.Origin = ""
	e.Marker = ""
	e.Operation = ""
	e.Error = nil
}
func status(e Edit) Object {
	// 冲突块仅返回前 100 个，完整列表通过冲突查询获取
	ranges, valid := conflicts(e.Content, e.Marker)
	count := len(ranges)
	if len(ranges) > 100 {
		ranges = ranges[:100]
	}
	out := Object{"edit_id": e.ID, "page_id": e.PageID, "title": e.Title, "kind": e.Kind, "state": e.State, "version": e.Version, "has_changes": e.Content != e.Origin, "close_requested": e.CloseRequested, "expires_at": e.ExpiresAt, "base_revision_id": e.BaseRevisionID, "content_hash": security.Hash(e.Content), "content_bytes": len(e.Content), "total_lines": len(lines(e.Content)), "conflict_count": count, "conflict_ranges": ranges, "conflict_markers_valid": valid, "resources": Object{"status": "wiki://edit/" + e.ID, "content": "wiki://edit/" + e.ID + "/content", "diff": "wiki://edit/" + e.ID + "/diff", "preview": "wiki://edit/" + e.ID + "/preview", "conflict": "wiki://edit/" + e.ID + "/conflict"}}
	if e.PageID == 0 {
		out["page_id"] = nil
	}
	if e.BaseRevisionID == 0 {
		out["base_revision_id"] = nil
	}
	if count > len(ranges) {
		out["conflicts_truncated"] = true
		out["next_tool"] = "wiki.get_edit_conflict"
	}
	if e.State == "loading" {
		out["loading"] = Object{"operation": e.Operation}
	}
	if e.Error != nil {
		out["error"] = e.Error
	}
	if e.State != "error" {
		out["content_source"] = e.Source
	}
	if e.ModerationBaseRevisionID != nil {
		out["moderation_base_revision_id"] = *e.ModerationBaseRevisionID
	}
	return out
}
func (s *Service) Begin(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	if x := require(c, "wiki:read"); x != nil {
		return nil, x
	}
	p := i.Page

	// 新建以标题定位目标页，并单独校验 create 权限
	if name == "begin_create" {
		p = PageRef{Title: i.Title}
		if x := require(c, "wiki:create"); x != nil {
			return nil, x
		}
	}
	snap, x := s.snapshotQuery(ctx, c, p, i.FollowRedirect, url.Values{"intestactionsdetail": {"full"}, "intestactions": {"edit|create"}})
	if x != nil {
		return nil, x
	}
	source := str(snap["content_source"])
	kind := "edit"
	if number(snap["page_id"]) == 0 {
		kind = "create"
	}

	// begin_create 要求页面不存在；已存在（含待审核）时给出区分错误
	if name == "begin_create" && source != "empty" {
		code := "PAGE_EXISTS"
		if source == "pending" {
			code = "PENDING_EDIT_EXISTS"
		}
		return nil, errCode(code, "The requested page already exists.")
	}
	if name == "begin_edit" && source == "empty" {
		return nil, errCode("NOT_FOUND", "Page was not found.")
	}
	if x = require(c, "wiki:"+kind); x != nil {
		return nil, x
	}

	// 页面内容与动作权限来自同一次上游查询。
	actions := object(snap["actions"])
	checkedActions := "edit"
	if kind == "create" {
		checkedActions = "edit|create"
	}
	// 任一请求动作被返回错误即视为无权限
	for _, action := range strings.Split(checkedActions, "|") {
		errors, ok := actions[action].([]any)
		if !ok || len(errors) != 0 {
			return nil, errCode("PERMISSION_DENIED", "Wiki denied the page editing permission.")
		}
	}
	now := time.Now().UTC()
	edit := Edit{ID: "edit_" + security.Random(), Owner: c.owner(c.Client.Config.SiteID), SiteID: c.Client.Config.SiteID, Kind: kind, PageID: number(snap["page_id"]), Title: str(snap["title"]), BaseRevisionID: number(snap["baseline_revision_id"]), Origin: str(snap["content"]), Content: str(snap["content"]), Source: source, Version: 1, State: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Duration(c.Config.App.Editing.TTL) * time.Second), StartTimestamp: now.Format("2006-01-02T15:04:05Z")}
	if c.Config.Static.Extensions.Moderation.Enabled {
		n := number(snap["temp_revision_id"])
		edit.ModerationBaseRevisionID = &n
	}
	// 事务内统计未结束会话并校验配额，避免并发创建突破上限
	x = s.Repo.tx(ctx, func(db *gorm.DB) error {
		if x := s.Repo.quota(db, edit.Owner); x != nil {
			return x
		}
		var count int64
		if x := db.Table("edit_sessions").Where("owner = ? AND state <> ? AND expires_at > ?", edit.Owner, "ended", now).Count(&count).Error; x != nil {
			return x
		}
		if count >= int64(c.Config.App.Editing.MaxActive) {
			return errCode("EDIT_QUOTA_EXCEEDED", "Active edit session quota is exceeded.")
		}
		return s.Repo.saveEdit(db, edit, true)
	})
	if x != nil {
		return nil, x
	}
	return status(edit), nil
}
func (s *Service) List(ctx context.Context, c *Call, i Input) (Object, error) {
	if x := require(c, "wiki:read"); x != nil {
		return nil, x
	}
	limit, x := positive(i.Limit, 20, 100)
	if x != nil {
		return nil, x
	}
	owner := c.owner(c.Client.Config.SiteID)
	cur, x := readCursor(i.Cursor, "list_edits", owner, fmt.Sprint(limit))
	if x != nil {
		return nil, x
	}
	var rows []Record
	if x = s.Repo.Store.DB.WithContext(ctx).Table("edit_sessions").Where("owner = ? AND state <> 'ended' AND expires_at > ?", owner, time.Now()).Order("id").Offset(cur.Offset).Limit(limit + 1).Find(&rows).Error; x != nil {
		return nil, x
	}
	var out []Object
	for _, row := range rows[:min(len(rows), limit)] {
		var p editPayload
		if x = s.Repo.Store.Vault.Open(row.Payload, &p, "edit_sessions:"+row.ID); x != nil {
			return nil, x
		}
		e := unpack(p)
		if e.EndedAt == nil {
			out = append(out, status(e))
		}
	}
	var next any
	if len(rows) > limit {
		next = cursor(cursorData{Tool: "list_edits", Owner: owner, Filter: fmt.Sprint(limit), Offset: cur.Offset + limit})
	}
	return Object{"edits": out, "next_cursor": next}, nil
}
func (s *Service) Get(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	if x := require(c, "wiki:read"); x != nil {
		return nil, x
	}
	e, x := s.Repo.edit(s.Repo.Store.DB.WithContext(ctx), i.EditID, false)
	if x != nil {
		return nil, x
	}
	if x = s.checked(e, c, i.ExpectedVersion, true); x != nil {
		return nil, x
	}
	// 状态查询不受占用影响，始终可读
	if name == "get_edit_status" {
		return status(e), nil
	}
	if e.State == "loading" {
		return nil, errCode("EDIT_SESSION_BUSY", "Edit session is busy. Read its status.")
	}

	// error 状态仅放行内容与搜索，其余操作被禁止
	if e.State == "error" && name != "get_edit_content" && name != "search_edit_content" {
		return nil, errCode("EDIT_SESSION_ERROR", "Only draft content, search and status can be read.")
	}
	switch name {
	case "get_edit_content":
		return content(e, c, i)
	case "search_edit_content":
		return search(e, c, i)
	case "get_edit_conflict":
		ranges, valid := conflicts(e.Content, e.Marker)
		limit, x := positive(i.Limit, 20, 100)
		if x != nil {
			return nil, x
		}
		cur, x := readCursor(i.Cursor, name, e.Owner, fmt.Sprint(e.ID, limit))
		if x != nil {
			return nil, x
		}

		// 游标携带版本，草稿更新后旧游标失效
		if cur.Version != 0 && cur.Version != e.Version {
			return nil, errCode("EDIT_SESSION_VERSION_MISMATCH", "Conflict cursor is stale.")
		}
		if cur.Offset < 0 || cur.Offset > len(ranges) {
			return nil, errCode("INVALID_CURSOR", "Conflict cursor is invalid.")
		}
		end := min(len(ranges), cur.Offset+limit)
		var next any
		if end < len(ranges) {
			next = cursor(cursorData{Tool: name, Owner: e.Owner, Filter: fmt.Sprint(e.ID, limit), Version: e.Version, Offset: end})
		}
		return Object{"edit_id": e.ID, "version": e.Version, "conflict_count": len(ranges), "conflict_markers_valid": valid, "conflict_ranges": ranges[cur.Offset:end], "next_cursor": next, "content_uri": "wiki://edit/" + e.ID + "/content"}, nil
	case "get_edit_diff":
		// 分页拉取时必须带上版本，当前版本比较还需目标 revision，防止分页期间基线漂移
		if i.Offset > 0 && (i.ExpectedVersion == 0 || i.Comparison == "current_to_edited" && i.ExpectedCurrentRevision == 0) {
			return nil, errCode("INVALID_REQUEST", "Diff pagination requires expected_version and, for current comparison, expected_current_revision.")
		}
		context := 3
		if i.ContextLines != nil {
			context = *i.ContextLines
		}
		if context < 0 || context > 100 {
			return nil, errCode("INVALID_REQUEST", "Invalid diff context.")
		}
		origin := e.Origin
		comparison := i.Comparison
		if comparison == "" {
			comparison = "origin_to_edited"
		}
		revision := int64(0)

		// 与当前线上版本比较时，需拉取指定 revision 的内容作为原侧
		if comparison == "current_to_edited" {
			if e.PageID == 0 {
				return nil, errCode("BASELINE_NOT_FOUND", "No public baseline is available.")
			}
			if x = s.site(c, e.SiteID); x != nil {
				return nil, x
			}
			var baseline Object
			revision = i.ExpectedCurrentRevision
			if revision == 0 {
				v, x := s.snapshot(ctx, c, PageRef{PageID: e.PageID}, false)
				if x != nil {
					return nil, x
				}
				revision = number(v["baseline_revision_id"])
				if v["content_source"] == "baseline" {
					baseline = v
				}
			}
			if revision <= 0 {
				return nil, errCode("BASELINE_NOT_FOUND", "No public baseline is available.")
			}
			r := baseline
			if r == nil {
				r, x = s.revision(ctx, c, revision)
				if x != nil {
					return nil, x
				}
			}
			if number(r["page_id"]) != e.PageID {
				return nil, errCode("INVALID_REQUEST", "Revision belongs to a different page.")
			}
			origin = str(r["content"])
		} else if comparison != "origin_to_edited" {
			return nil, errCode("INVALID_REQUEST", "Invalid diff comparison.")
		}
		diff := unified(origin, e.Content, context)
		f, x := fragment(diff, i.Offset, i.Limit, c.Config.App.Results.TextMax)
		if x != nil {
			return nil, x
		}

		// fragment 返回的 content 即 diff 文本，重命名以匹配接口字段
		f["diff"] = f["content"]
		delete(f, "content")
		f["version"] = e.Version
		f["edited_version"] = e.Version
		f["comparison"] = comparison
		f["current_revision_id"] = revision
		f["content_hash"] = security.Hash(diff)
		f["edit_id"] = e.ID
		f["origin_revision_id"] = e.BaseRevisionID
		f["base_revision_id"] = e.BaseRevisionID
		f["context_lines"] = context
		return f, nil
	}
	return nil, errCode("INVALID_REQUEST", "Unknown edit read operation.")
}
func content(e Edit, c *Call, i Input) (Object, error) {
	out := Object{"edit_id": e.ID, "version": e.Version, "content_hash": security.Hash(e.Content)}
	if e.State != "error" {
		out["content_source"] = e.Source
	}
	// 字节分片与行分片两种读取方式互斥
	if i.Offset != 0 || i.Limit != 0 {
		if i.StartLine != 0 || i.LineCount != 0 || i.IncludeLineNumbers != nil {
			return nil, errCode("INVALID_REQUEST", "Byte and line inputs are mutually exclusive.")
		}
		f, x := fragment(e.Content, i.Offset, i.Limit, c.Config.App.Results.TextMax)
		if x != nil {
			return nil, x
		}
		for k, v := range f {
			out[k] = v
		}
		return out, nil
	}
	a := lines(e.Content)
	start := i.StartLine
	if start == 0 {
		start = 1
	}
	count, x := positive(i.LineCount, 200, 2000)
	if x != nil {
		return nil, x
	}
	// 允许空文档的第一行作为唯一合法起点
	if start < 1 || start > len(a) && !(len(a) == 0 && start == 1) {
		return nil, errCode("LINE_RANGE_INVALID", "Requested line is outside the draft.")
	}
	end := min(len(a), start-1+count)
	var b strings.Builder
	actual := start - 1
	numbered := i.IncludeLineNumbers == nil || *i.IncludeLineNumbers

	// 逐行累积，按返回字节上限截断；单行超限且尚无内容时给出字节读取指引
	for k := start - 1; k < end; k++ {
		v := a[k]
		if numbered {
			v = fmt.Sprintf("%d | %s", k+1, v)
		}
		extra := len(v)
		if b.Len() > 0 {
			extra++
		}
		if int64(b.Len()+extra) > c.Config.App.Results.TextMax {
			if b.Len() == 0 {
				return nil, detail("LINE_TOO_LONG", "Read this line using byte offset and limit.", Object{"next_tool": "wiki.get_edit_content", "offset": len(strings.Join(a[:k], "\n")) + min(k, 1)})
			}
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(v)
		actual = k + 1
	}
	var next any
	if actual < len(a) {
		next = actual + 1
	}
	out["content"] = b.String()
	out["start_line"] = start
	out["end_line"] = actual
	out["total_lines"] = len(a)
	out["returned_lines"] = max(0, actual-start+1)
	out["next_line"] = next
	out["truncated"] = actual < len(a)
	return out, nil
}
func search(e Edit, c *Call, i Input) (Object, error) {
	if i.Pattern == "" || len(i.Pattern) > 4096 {
		return nil, errCode("INVALID_SEARCH_PATTERN", "Search pattern must be nonempty and bounded.")
	}
	sensitive := i.CaseSensitive == nil || *i.CaseSensitive
	context := 3
	if i.ContextLines != nil {
		context = *i.ContextLines
	}
	if context < 0 || context > 100 {
		return nil, errCode("INVALID_REQUEST", "Invalid search context.")
	}
	limit, x := positive(i.MaxMatches, 20, 100)
	if x != nil {
		return nil, x
	}
	var re *regexp.Regexp

	// 正则模式编译；不区分大小写时通过 (?i) 前缀实现
	if i.Regex {
		pattern := i.Pattern
		if !sensitive {
			pattern = "(?i)" + pattern
		}
		re, x = regexp.Compile(pattern)
		if x != nil {
			return nil, errCode("INVALID_SEARCH_PATTERN", "Pattern is not supported by Go's linear time regex engine.")
		}
	}

	// 游标指纹绑定搜索条件，条件变化后旧游标失效
	filter := fingerprint(Object{"edit": e.ID, "pattern": i.Pattern, "regex": i.Regex, "sensitive": sensitive, "context": context, "limit": limit})
	cur, x := readCursor(i.Cursor, "search_edit_content", e.Owner, filter)
	if x != nil {
		return nil, x
	}
	if cur.Version != 0 && cur.Version != e.Version {
		return nil, errCode("EDIT_SESSION_VERSION_MISMATCH", "Search cursor is stale.")
	}
	a := lines(e.Content)
	if cur.Offset < 0 || cur.Offset > len(a) {
		return nil, errCode("INVALID_CURSOR", "Search cursor is invalid.")
	}
	var results []Object
	var ranges []LineRange
	nextLine := 0

	// 上下文与匹配摘要共享文本预算，预算按一半上限预留
	budget := int(c.Config.App.Results.TextMax) / 2
	for k := cur.Offset; k < len(a); k++ {
		match := false
		if re != nil {
			match = re.MatchString(a[k])
		} else if sensitive {
			match = strings.Contains(a[k], i.Pattern)
		} else {
			match = strings.Contains(strings.ToLower(a[k]), strings.ToLower(i.Pattern))
		}
		if !match {
			continue
		}
		if len(results) >= limit || budget <= 0 {
			nextLine = k
			break
		}
		summary := a[k]
		incomplete := len(summary) > 1024
		if incomplete {
			f, _ := fragment(summary, 0, 1024, 1024)
			summary = str(f["content"])
		}
		results = append(results, Object{"line": k + 1, "text": summary, "incomplete": incomplete})
		budget -= len(summary) + 100

		// 相邻或重叠的上下文区间合并，减少返回片段数
		r := LineRange{max(1, k+1-context), min(len(a), k+1+context)}
		if len(ranges) > 0 && r.Start <= ranges[len(ranges)-1].End+1 {
			ranges[len(ranges)-1].End = r.End
		} else {
			ranges = append(ranges, r)
		}
	}
	var contexts []Object
	for _, r := range ranges {
		t := strings.Join(a[r.Start-1:r.End], "\n")
		incomplete := len(t) > max(0, budget)
		if incomplete {
			if budget > 0 {
				f, _ := fragment(t, 0, budget, int64(budget))
				t = str(f["content"])
			} else {
				t = ""
			}
		}
		budget -= len(t)
		contexts = append(contexts, Object{"start_line": r.Start, "end_line": r.End, "content": t, "incomplete": incomplete})
	}
	var next any
	if nextLine > 0 {
		next = cursor(cursorData{Tool: "search_edit_content", Owner: e.Owner, Filter: filter, Offset: nextLine, Version: e.Version})
	}
	return Object{"edit_id": e.ID, "version": e.Version, "search_results": results, "contexts": contexts, "truncated": next != nil, "next_cursor": next}, nil
}
func (s *Service) Update(ctx context.Context, c *Call, i Input) (Object, error) {
	if i.BaseVersion <= 0 || len(i.Operations) == 0 || len(i.Operations) > c.Config.App.Editing.MaxOperations {
		return nil, errCode("INVALID_REQUEST", "A positive base_version and bounded operations are required.")
	}
	var out Object
	x := s.Repo.tx(ctx, func(db *gorm.DB) error {
		e, x := s.Repo.edit(db, i.EditID, true)
		if x != nil {
			return x
		}
		if x = s.checked(e, c, i.BaseVersion, false); x != nil {
			return x
		}
		// 仅 ready 状态可编辑，避免与进行中的提交冲突
		if e.State != "ready" {
			return errCode("EDIT_SESSION_BUSY", "Only ready sessions can be updated.")
		}
		before := e.Content
		text, x := Apply(e.Content, i.Operations, c.Config.App.Editing.MaxContent)
		if x != nil {
			return x
		}

		// 内容有实际变化才推进版本号
		changed := text != e.Content
		if changed {
			e.Content = text
			e.Version++
		}
		if x = s.Repo.saveEdit(db, e, false); x != nil {
			return x
		}
		out = status(e)
		out["changed"] = changed
		out["operations_applied"] = len(i.Operations)

		// 计算变更行范围，转成 1 基行号并限制数量
		changedRanges := []LineRange{}
		for _, r := range changes(lines(before), lines(text)) {
			if r.BEnd > r.BStart {
				changedRanges = append(changedRanges, LineRange{r.BStart + 1, r.BEnd})
			}
		}
		out["changed_ranges"] = changedRanges[:min(len(changedRanges), 100)]
		return nil
	})
	return out, x
}
