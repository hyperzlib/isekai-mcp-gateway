package wiki

import (
	"context"
	"fmt"
	"isekai-mcp-gateway/internal/failure"
	"net/url"
)

func (s *Service) Mutation(ctx context.Context, c *Call, name string, i Input) (Object, error) {
	scope := "wiki:move"
	action := "move"

	// 删除复用同一入口，仅切换动作与所需权限
	if name == "delete_page" {
		scope = "wiki:delete"
		action = "delete"
	}
	if x := require(c, "wiki:read", scope); x != nil {
		return nil, x
	}
	snap, x := s.snapshot(ctx, c, i.Page, false)
	if x != nil {
		return nil, x
	}
	id := number(snap["page_id"])
	if id <= 0 {
		return nil, errCode("NOT_FOUND", "Public page was not found.")
	}
	params := Object{"siteId": c.Client.Config.SiteID, "page_id": id, "reason": i.Reason}
	q := url.Values{"action": {action}, "pageid": {fmt.Sprint(id)}, "reason": {i.Reason}}

	// move 需校验目标标题合法，并用 Wiki 归一化后的标题作为实际目标
	if action == "move" {
		if _, x = pageParams(PageRef{Title: i.ToTitle}); x != nil {
			return nil, x
		}
		q.Set("fromid", fmt.Sprint(id))
		q.Del("pageid")
		q.Set("to", i.ToTitle)
		target, x := c.Client.Query(ctx, c.Credentials, url.Values{"action": {"query"}, "prop": {"info"}, "titles": {i.ToTitle}})
		if x != nil {
			return nil, x
		}
		a, x := pages(target)
		if x != nil {
			return nil, x
		}
		normalized := str(object(a[0])["title"])
		if normalized == "" {
			return nil, errCode("UPSTREAM_RESPONSE_INVALID", "Wiki did not normalize the move target.")
		}
		q.Set("to", normalized)
		params["to_title"] = normalized
	}
	ch, x := s.policy(ctx, c, action, name, id, params, i.ChallengeResponses, zeroTime)
	if x != nil {
		return nil, x
	}
	token, x := c.Client.CSRF(ctx, c.Credentials)
	if x != nil {
		_ = s.finishChallenge(context.WithoutCancel(ctx), ch, "ready")
		return nil, x
	}
	q.Set("token", token)
	v, x := c.Client.Write(ctx, c.Credentials, q)

	// 挑战最终状态：成功消费，可重试错误重回 ready，网络异常则冻结并标记结果未知
	state := "consumed"
	if x != nil {
		state = "ready"
		if failure.Describe(x).Code == "API_NETWORK_ERROR" {
			state = "frozen"
			x = detail("MUTATION_RESULT_UNKNOWN", "The write result is unknown. Inspect the page before any further write.", Object{"reason": "API_NETWORK_ERROR"})
		}
	}
	if y := s.finishChallenge(context.WithoutCancel(ctx), ch, state); x == nil {
		x = y
	}
	if x != nil {
		return nil, x
	}
	// 上游未返回对应动作结果时，无法确认变更是否生效
	out := object(v[action])
	if len(out) == 0 {
		return nil, errCode("MUTATION_RESULT_UNKNOWN", "Wiki did not return a recognized result. Inspect the page.")
	}
	return out, nil
}
