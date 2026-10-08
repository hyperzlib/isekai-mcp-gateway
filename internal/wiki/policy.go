package wiki

import (
	"context"
	"gorm.io/gorm"
	"isekai-mcp-gateway/internal/security"
	"time"
)

// policyFingerprint 绑定配置中的 additions 与 extensions，配置变化即视为策略变化。
func policyFingerprint(c *Call) string {
	return fingerprint(Object{"additions": c.Config.App.Additions, "extensions": c.Config.Static.Extensions})
}
func (s *Service) confirmation(ctx context.Context, c *Call, tool string, params Object, responses []Response, expiry time.Time) (string, error) {
	fp := policyFingerprint(c)
	owner := c.principalKey()
	var selected string

	// 每个响应必须带 ID 且 confirmed=true，且一次操作只允许一个确认。
	for _, r := range responses {
		if !r.Confirmed || r.ChallengeID == "" {
			return "", errCode("INVALID_REQUEST", "Challenge responses require an ID and confirmed=true.")
		}
		if selected != "" {
			return "", errCode("INVALID_REQUEST", "Only one confirmation is required for this operation.")
		}
		selected = r.ChallengeID
	}
	if selected != "" {
		acquired := false

		// 在事务中独占校验并消费挑战，成功即标记 reserved 防止重放。
		x := s.Repo.tx(ctx, func(db *gorm.DB) error {
			var ch Challenge

			// 加锁读取，避免并发确认同一挑战。
			row, x := s.Repo.load(db, "challenges", selected, &ch, true)
			if x != nil {
				return x
			}

			// 挑战归属与工具必须匹配当前调用者。
			if ch.Owner != owner || ch.Tool != tool {
				return errCode("CHALLENGE_INVALID", "Confirmation belongs to another caller or tool.")
			}

			// 状态、有效期与参数指纹任一不符则挑战不可用。
			if ch.State != "ready" || !ch.ExpiresAt.After(time.Now()) || ch.Fingerprint != fp || fingerprint(ch.Params) != fingerprint(params) {
				// 已就绪但不匹配的挑战标记失效，防止继续使用。
				if ch.State == "ready" {
					ch.State = "invalid"
					row.State = ch.State
					if x = s.Repo.save(db, "challenges", row, ch, false); x != nil {
						return x
					}
				}
				return nil
			}

			// 标记为 reserved，表示确认已被本次调用占用。
			ch.State = "reserved"
			row.State = ch.State
			acquired = true
			return s.Repo.save(db, "challenges", row, ch, false)
		})
		if x != nil {
			return "", x
		}
		if acquired {
			return selected, nil
		}
	}

	// 挑战有效期取配置 TTL 与调用方 deadline 的较早者。
	expiry2 := time.Now().UTC().Add(time.Duration(c.Config.App.Additions.Foreign.TTL) * time.Second)
	if !expiry.IsZero() && expiry.Before(expiry2) {
		expiry2 = expiry
	}

	// 未提供有效确认时新建挑战并返回给调用方。
	ch := Challenge{ID: "challenge_" + security.Random(), Owner: owner, Tool: tool, Fingerprint: fp, State: "ready", Params: params, ExpiresAt: expiry2}
	row := Record{ID: ch.ID, Owner: owner, State: ch.State, ExpiresAt: expiry2}
	if x := s.Repo.save(s.Repo.Store.DB.WithContext(ctx), "challenges", row, ch, true); x != nil {
		return "", x
	}
	return "", detail("POLICY_CHALLENGE", "Ask the user to confirm this operation, then call the same tool with challenge_responses.", Object{"challenge_id": ch.ID, "policy_id": tool, "policy_version": c.Config.App.Additions.Foreign.Version, "tool": "wiki." + tool, "confirmation_params": params, "expires_at": expiry2, "required_help": "wiki://help/confirmation", "required_action": "user_confirmation"})
}
func (s *Service) policy(ctx context.Context, c *Call, operation, tool string, pageID int64, params Object, responses []Response, expiry time.Time) (string, error) {
	// 仅当操作在配置的外站操作列表中且页面存在时才需要确认。
	enabled := false
	for _, v := range c.Config.App.Additions.Foreign.Operations {
		if v == operation {
			enabled = true
		}
	}
	if !enabled || pageID <= 0 {
		return "", nil
	}
	edited, x := s.contribution(ctx, c, pageID)
	if x != nil {
		return "", x
	}

	// 用户已编辑过的页面跳过确认。
	if edited {
		return "", nil
	}
	return s.confirmation(ctx, c, tool, params, responses, expiry)
}
func (s *Service) finishChallenge(ctx context.Context, id, state string) error {
	// 无挑战 ID 时无需处理。
	if id == "" {
		return nil
	}
	return s.Repo.tx(ctx, func(db *gorm.DB) error {
		var ch Challenge
		row, x := s.Repo.load(db, "challenges", id, &ch, true)
		if x != nil {
			return x
		}

		// 仅处理占用中或冻结的挑战，避免覆盖终态。
		if ch.State != "reserved" && ch.State != "frozen" {
			return nil
		}
		ch.State = state
		row.State = state
		return s.Repo.save(db, "challenges", row, ch, false)
	})
}
