package wiki

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/security"
	"net/url"
	"time"
)

func (s *Service) occupy(ctx context.Context, c *Call, i Input, op string) (Edit, error) {
	var edit Edit

	// base_version 是乐观锁凭证，缺失时无法定位草稿版本
	if i.BaseVersion <= 0 {
		return edit, errCode("INVALID_REQUEST", "base_version is required.")
	}

	// 在事务内完成鉴权、状态校验与占用，避免并发操作同时进入
	x := s.Repo.tx(ctx, func(db *gorm.DB) error {
		e, x := s.Repo.edit(db, i.EditID, true)
		if x != nil {
			return x
		}
		if x = s.checked(e, c, i.BaseVersion, op == "preview"); x != nil {
			return x
		}
		if x = require(c, "wiki:read"); x != nil {
			return x
		}
		// loading 表示已有操作占用该会话，不允许并发进入
		if e.State == "loading" {
			return errCode("EDIT_SESSION_BUSY", "Edit session is busy.")
		}

		// error 状态会话不可再操作；commit 仅允许从 ready 发起
		if e.State == "error" || op == "commit" && e.State != "ready" {
			return errCode("EDIT_SESSION_ERROR", "This operation is unavailable in the current state.")
		}

		// preview 与 commit 前必须无未解决的冲突标记
		if op != "pull" {
			ranges, valid := conflicts(e.Content, e.Marker)
			if len(ranges) > 0 || !valid {
				return errCode("UNRESOLVED_CONFLICT", "Resolve all conflict markers before preview or commit.")
			}
		}

		// 记录原状态并置为 loading，实现会话占用
		e.PreviousState = e.State
		e.State = "loading"
		e.Operation = op
		edit = e
		return s.Repo.saveEdit(db, e, false)
	})
	return edit, x
}
func (s *Service) complete(ctx context.Context, e Edit, fn func(*Edit) error) error {
	return s.Repo.tx(context.WithoutCancel(ctx), func(db *gorm.DB) error {
		current, x := s.Repo.edit(db, e.ID, true)
		if x != nil {
			return x
		}

		// 校验该操作仍持有占用，防止被后续更新或其它操作覆盖
		if current.State != "loading" || current.Operation != e.Operation || current.Version != e.Version {
			return errCode("EDIT_SESSION_VERSION_MISMATCH", "The operation no longer owns this session.")
		}
		if x = fn(&current); x != nil {
			return x
		}

		// 释放占用；若期间收到关闭请求则直接结束会话
		current.Operation = ""
		if current.CloseRequested {
			endEdit(&current)
		}
		return s.Repo.saveEdit(db, current, false)
	})
}
func sessionError(code, message string) Object {
	return Object{"errorCode": code, "errorMessage": message, "recommandAction": "Read status and call wiki.pull_edit or close the session."}
}
func (s *Service) Commit(ctx context.Context, c *Call, i Input) (Object, error) {
	// ai_involvement 决定写入时附加的标签，必须是受支持的枚举值
	if i.AIInvolvement != "none" && i.AIInvolvement != "ai_organized" && i.AIInvolvement != "ai_assisted" && i.AIInvolvement != "ai_generated" {
		return nil, errCode("INVALID_REQUEST", "ai_involvement must be none, ai_organized, ai_assisted or ai_generated.")
	}

	// 幂等：同一版本已成功提交且会话未被标记 unsync 时，直接返回缓存结果
	if old, x := s.Repo.edit(s.Repo.Store.DB.WithContext(ctx), i.EditID, false); x == nil && old.Owner == c.owner(old.SiteID) && old.ExpiresAt.After(time.Now()) && old.State != "error" {
		if x = require(c, "wiki:read", "wiki:"+old.Kind); x != nil {
			return nil, x
		}
		var a Attempt
		if _, x = s.Repo.load(s.Repo.Store.DB.WithContext(ctx), "commit_attempts", fmt.Sprintf("%s:%d", old.ID, i.BaseVersion), &a, false); x == nil && a.Result != nil && old.State != "unsync" {
			return a.Result, nil
		}
	}

	// 占用会话，锁定版本后再向上游发起写入
	edit, x := s.occupy(ctx, c, i, "commit")
	if x != nil {
		return nil, x
	}

	// 站点不可用时回滚占用，保持原状态
	if x = s.site(c, edit.SiteID); x != nil {
		_ = s.complete(ctx, edit, func(e *Edit) error { e.State = e.PreviousState; return nil })
		return nil, x
	}
	challenge := ""
	sent := false
	attemptID := fmt.Sprintf("%s:%d", edit.ID, edit.Version)

	// 失败降级：区分请求未发出与已发出但结果未知，后者需冻结以便恢复
	abort := func(cause error) (Object, error) {
		code := failure.Describe(cause).Code
		state := "ready"

		// 请求已发出或网络异常时，本地状态无法确认，标记为 unsync 等待恢复
		if sent || code == "API_NETWORK_ERROR" {
			state = "unsync"
		}
		reason := "save_failed"
		if sent && (code == "API_NETWORK_ERROR" || code == "UPSTREAM_RESPONSE_INVALID") {
			reason = "result_unknown"
		}
		chstate := "ready"
		if reason == "result_unknown" {
			chstate = "frozen"
		}
		if sent {
			if x := s.Repo.tx(context.WithoutCancel(ctx), func(db *gorm.DB) error {
				var a Attempt
				row, x := s.Repo.load(db, "commit_attempts", attemptID, &a, true)
				if x != nil {
					return x
				}
				a.Reason = reason
				row.State = reason
				return s.Repo.save(db, "commit_attempts", row, a, false)
			}); x != nil {
				return nil, x
			}
		}

		// 恢复挑战与会话状态，并把原始错误上抛
		_ = s.finishChallenge(context.WithoutCancel(ctx), challenge, chstate)
		saveErr := s.complete(ctx, edit, func(e *Edit) error {
			e.State = state
			if state == "unsync" {
				e.Error = sessionError(reason, "Submission needs recovery. Call wiki.pull_edit.")
			}
			return nil
		})
		if saveErr != nil {
			return nil, saveErr
		}
		return nil, cause
	}
	params := Object{"siteId": edit.SiteID, "edit_id": edit.ID, "kind": edit.Kind, "page_id": edit.PageID, "title": edit.Title}
	challenge, x = s.policy(ctx, c, "commit", "commit_edit", edit.PageID, params, i.ChallengeResponses, edit.ExpiresAt)
	if x != nil {
		return abort(x)
	}
	token, x := c.Client.CSRF(ctx, c.Credentials)
	if x != nil {
		return abort(x)
	}
	q := url.Values{"action": {"edit"}, "text": {edit.Content}, "summary": {i.Summary}, "token": {token}, "starttimestamp": {edit.StartTimestamp}}

	// create 与 edit 使用不同的防冲突参数，避免误建或覆盖他人编辑
	if edit.Kind == "create" {
		q.Set("title", edit.Title)
		q.Set("createonly", "1")
	} else {
		q.Set("pageid", fmt.Sprint(edit.PageID))
		q.Set("baserevid", fmt.Sprint(edit.BaseRevisionID))
		q.Set("nocreate", "1")
	}
	if edit.ModerationBaseRevisionID != nil {
		q.Set("moderation_baserevid", fmt.Sprint(*edit.ModerationBaseRevisionID))
	}
	if tag := c.Config.App.Additions.Tags.Mapping[i.AIInvolvement]; tag != "" {
		q.Set("tags", tag)
	}
	// 写入前先落库提交尝试记录，确保请求发出后可据此恢复
	x = s.Repo.tx(ctx, func(db *gorm.DB) error {
		current, x := s.Repo.edit(db, edit.ID, true)
		if x != nil {
			return x
		}
		if current.State != "loading" || current.Version != edit.Version {
			return errCode("EDIT_SESSION_BUSY", "Submission lost its session occupancy.")
		}

		// 提交期间收到关闭请求，则放弃写入并结束会话
		if current.CloseRequested {
			endEdit(&current)
			return s.Repo.saveEdit(db, current, false)
		}
		a := Attempt{EditID: edit.ID, Version: edit.Version, Sent: true, ChallengeID: challenge}
		row := Record{ID: attemptID, Owner: edit.Owner, State: "sending", Version: edit.Version, ExpiresAt: edit.ExpiresAt}
		if x = s.Repo.save(db, "commit_attempts", row, a, true); x != nil {
			return x
		}
		sent = true
		return nil
	})
	if x != nil {
		return abort(x)
	}
	if !sent {
		_ = s.finishChallenge(context.WithoutCancel(ctx), challenge, "ready")
		return nil, errCode("EDIT_SESSION_CLOSED", "Session closed before the save request was sent.")
	}
	// Clear all users' cached snapshots once a write may have reached MediaWiki.
	defer s.invalidatePageCache(c, edit.PageID, edit.Title)
	v, x := c.Client.Write(ctx, c.Credentials, q)
	if x != nil {
		return abort(x)
	}
	ed := object(v["edit"])
	mod := object(ed["moderation"])
	pending := false
	outcome := "saved"
	changed := true

	// MediaWiki 对进入审核队列的编辑以 error.code 返回，需单独识别
	if er := object(v["error"]); er["code"] == "moderation-edit-queued" {
		pending = true
		mod = object(er["moderation"])
		outcome = "submitted_for_review"
	} else {
		if ed["result"] != "Success" {
			if ed["result"] == "Failure" {
				return abort(&mediawiki.APIError{Code: str(ed["code"])})
			}
			return abort(errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not return a recognized save result."))
		}
		// nochange 表示内容与当前版本一致，未产生新修订
		if value, ok := ed["nochange"]; ok && value != false {
			changed = false
			outcome = "nochange"
		}
		if len(mod) > 0 {
			pending = true
		}
	}
	// 组装返回结果：nochange 时 revision 回退为基线，页面 ID 缺失时沿用草稿值
	result := Object{"edit_id": edit.ID, "outcome": outcome, "url": edit.URL, "changed": changed, "revision_id": nil, "content_source": "baseline", "version": edit.Version + 1}
	revision := number(ed["newrevid"])
	if !changed && revision == 0 {
		revision = edit.BaseRevisionID
	}
	pageID := number(ed["pageid"])
	if pageID == 0 {
		pageID = edit.PageID
	}

	// 待审核编辑公开内容取 pending，并附上审核链相关字段
	if pending {
		result["content_source"] = "pending"
		for _, k := range []string{"mod_id", "temp_revision_id", "chain_version"} {
			result[k] = mod[k]
		}
	} else {
		result["revision_id"] = revision
	}
	if title := str(ed["title"]); title != "" {
		result["title"] = title
	} else {
		result["title"] = edit.Title
	}
	result["page_id"] = pageID
	if pageID == 0 {
		result["page_id"] = nil
	}
	if !pending {
		result["base_revision_id"] = revision
		result["has_changes"] = false
		if edit.ModerationBaseRevisionID != nil {
			result["moderation_base_revision_id"] = 0
		}
	}
	// 先持久化上游已知结果，便于恢复时区分本地同步事务丢失与响应丢失
	// First persist the known outcome. Recovery can distinguish a lost local
	// synchronization transaction from a lost MediaWiki response.
	x = s.Repo.tx(context.WithoutCancel(ctx), func(db *gorm.DB) error {
		var a Attempt
		row, x := s.Repo.load(db, "commit_attempts", attemptID, &a, true)
		if x != nil {
			return x
		}
		a.Result = result
		row.State = "known_success"
		return s.Repo.save(db, "commit_attempts", row, a, false)
	})
	if x != nil {
		return nil, x
	}
	// The confirmed write is durable before this optional metadata refresh.
	pageURL, urlErr := s.pageURL(ctx, c, PageRef{PageID: pageID, Title: str(result["title"])})
	if urlErr != nil {
		result["url_refresh_warning"] = Object{"code": "URL_REFRESH_FAILED", "message": "Wiki accepted the submission, but the page URL could not be refreshed. Do not resubmit; use wiki.pull_edit for an open session."}
	} else {
		result["url"] = pageURL
	}
	// Persist refreshed metadata for idempotent reads of this submission.
	x = s.Repo.tx(context.WithoutCancel(ctx), func(db *gorm.DB) error {
		var a Attempt
		row, err := s.Repo.load(db, "commit_attempts", attemptID, &a, true)
		if err != nil {
			return err
		}
		a.Result = result
		return s.Repo.save(db, "commit_attempts", row, a, false)
	})
	if x != nil {
		_ = s.complete(ctx, edit, func(e *Edit) error {
			e.State = "unsync"
			e.Error = sessionError("saved_sync_failed", "Wiki saved the edit, but local URL synchronization failed.")
			return nil
		})
		return nil, x
	}
	// 非待审核编辑若缺少 revision 或 pageID，说明基线同步不完整
	syncFailed := !pending && (revision <= 0 || pageID <= 0)
	x = s.complete(ctx, edit, func(e *Edit) error {
		e.URL = str(result["url"])
		if syncFailed {
			e.State = "unsync"
			e.Error = sessionError("saved_sync_failed", "Wiki confirmed saving, but the public baseline is incomplete.")
			return nil
		}
		e.Version++
		e.Error = nil

		// 待审核编辑本地直接结束；已保存编辑则刷新基线字段回到 ready
		if pending {
			endEdit(e)
			e.EndReason = "committed"
		} else {
			e.PageID = pageID
			e.Title = str(result["title"])
			e.BaseRevisionID = revision
			if e.ModerationBaseRevisionID != nil {
				z := int64(0)
				e.ModerationBaseRevisionID = &z
			}
			e.Kind = "edit"
			e.Origin = e.Content
			e.Source = "baseline"
			e.Marker = ""
			e.State = "ready"
			result["base_revision_id"] = revision
			result["has_changes"] = false
			if e.ModerationBaseRevisionID != nil {
				result["moderation_base_revision_id"] = 0
			}
		}
		return nil
	})
	if x != nil {
		// 上游结果已确定，同步失败不能让会话一直处于占用，标记 unsync 等待恢复
		// The upstream outcome is durable; a transient synchronization failure
		// must not leave the session occupied until the next process restart.
		_ = s.complete(ctx, edit, func(e *Edit) error {
			e.State = "unsync"
			e.Error = sessionError("saved_sync_failed", "Wiki saved the edit, but local baseline synchronization failed.")
			return nil
		})
		return nil, x
	}
	if x = s.finishChallenge(context.WithoutCancel(ctx), challenge, "consumed"); x != nil {
		return nil, x
	}
	if syncFailed {
		return nil, errCode("SAVED_SYNC_FAILED", "Wiki saved the edit. Call wiki.pull_edit to recover its baseline.")
	}
	return result, nil
}
func (s *Service) Pull(ctx context.Context, c *Call, i Input) (Object, error) {
	e, x := s.occupy(ctx, c, i, "pull")
	if x != nil {
		return nil, x
	}
	// restore 在失败时把会话状态回滚到占用前
	restore := func(cause error) (Object, error) {
		x := s.complete(ctx, e, func(v *Edit) error { v.State = v.PreviousState; return nil })
		if x != nil {
			return nil, x
		}
		return nil, cause
	}
	if x = s.site(c, e.SiteID); x != nil {
		return restore(x)
	}
	// 有 pageID 时按 ID 拉取，否则按标题定位
	p := PageRef{PageID: e.PageID}
	if e.PageID == 0 {
		p = PageRef{Title: e.Title}
	}
	v, x := s.snapshot(ctx, c, p, false)
	if x != nil {
		return restore(x)
	}
	id := number(v["page_id"])

	// 原页面被删除或标题指向了不同页面，视为身份丢失
	if e.PageID > 0 && id != e.PageID {
		x = s.complete(ctx, e, func(v *Edit) error {
			v.State = "error"
			v.Version++
			v.Error = sessionError("PAGE_IDENTITY_LOST", "The original page no longer exists.")
			return nil
		})
		if x != nil {
			return nil, x
		}
		return nil, errCode("PAGE_IDENTITY_LOST", "The original page no longer exists.")
	}
	// 无 pageID 说明目标页面不存在，按新建处理
	kind := "edit"
	if id == 0 {
		kind = "create"
	}
	if x = require(c, "wiki:"+kind); x != nil {
		return restore(x)
	}
	current := str(v["content"])
	text := e.Content
	marker := ""
	outcome := "content_already_present"

	// 草稿与远端内容不一致时，用随机标记生成三方冲突块
	if text != current {
		marker = "pull_" + security.Random()
		text = pullConflict(current, text, marker)
		outcome = "conflicts_created"
	}
	if int64(len(text)) > c.Config.App.Editing.MaxContent {
		return restore(errCode("CONTENT_TOO_LARGE", "Conflict draft exceeds the content limit."))
	}
	var result Object
	x = s.complete(ctx, e, func(edit *Edit) error {
		edit.Content = text
		edit.Origin = current
		edit.Marker = marker
		edit.PageID = id
		edit.Title = str(v["title"])
		edit.URL = str(v["url"])
		edit.BaseRevisionID = number(v["baseline_revision_id"])
		if edit.ModerationBaseRevisionID != nil {
			n := number(v["temp_revision_id"])
			edit.ModerationBaseRevisionID = &n
		}
		edit.Source = str(v["content_source"])
		edit.Kind = kind
		edit.Version++
		edit.State = "ready"
		edit.Error = nil
		result = status(*edit)
		result["outcome"] = outcome
		return nil
	})
	return result, x
}
func (s *Service) Preview(ctx context.Context, c *Call, i Input) (Object, error) {
	e, x := s.occupy(ctx, c, i, "preview")
	if x != nil {
		return nil, x
	}
	if x = s.site(c, e.SiteID); x == nil {
		var v Object
		v, x = c.Client.Query(ctx, c.Credentials, url.Values{"action": {"parse"}, "title": {e.Title}, "text": {e.Content}, "contentmodel": {"wikitext"}, "prop": {"text"}})
		if x == nil {
			html := str(object(v["parse"])["text"])

			// 预览 HTML 超限时不返回内容，仅保留错误
			if int64(len(html)) > c.Config.App.Results.PreviewMax {
				x = errCode("PREVIEW_TOO_LARGE", "Preview exceeds the HTML limit.")
			} else {
				out := Object{"edit_id": e.ID, "version": e.Version, "html": html, "content_hash": security.Hash(html)}
				x = s.complete(ctx, e, func(v *Edit) error { v.State = v.PreviousState; return nil })
				return out, x
			}
		}
	}
	finish := s.complete(ctx, e, func(v *Edit) error { v.State = v.PreviousState; return nil })
	if finish != nil {
		return nil, finish
	}
	return nil, x
}
func (s *Service) Close(ctx context.Context, c *Call, i Input) (Object, error) {
	if i.BaseVersion <= 0 {
		return nil, errCode("INVALID_REQUEST", "base_version is required.")
	}
	e, x := s.Repo.edit(s.Repo.Store.DB.WithContext(ctx), i.EditID, false)
	if x != nil {
		return nil, x
	}
	if x = s.checked(e, c, i.BaseVersion, false); x != nil {
		return nil, x
	}
	challenge := ""

	// 草稿有未保存改动时，关闭需二次确认以防误丢内容
	if e.Content != e.Origin {
		challenge, x = s.confirmation(ctx, c, "close_edit", Object{"siteId": e.SiteID, "edit_id": e.ID, "version": e.Version}, i.ChallengeResponses, e.ExpiresAt)
		if x != nil {
			return nil, x
		}
	}
	var result Object
	x = s.Repo.tx(ctx, func(db *gorm.DB) error {
		e, x := s.Repo.edit(db, i.EditID, true)
		if x != nil {
			return x
		}
		if x = s.checked(e, c, i.BaseVersion, false); x != nil {
			return x
		}

		// 会话正被其它操作占用时只登记关闭请求，待其完成后再结束
		if e.State == "loading" {
			e.CloseRequested = true
		} else {
			endEdit(&e)
		}
		result = Object{"edit_id": e.ID, "close_requested": e.CloseRequested, "closed": e.EndedAt != nil, "version": e.Version}
		return s.Repo.saveEdit(db, e, false)
	})
	state := "consumed"
	if x != nil {
		state = "ready"
	}
	if y := s.finishChallenge(context.WithoutCancel(ctx), challenge, state); x == nil {
		x = y
	}
	return result, x
}
