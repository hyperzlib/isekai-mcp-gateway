package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

// AttachSession 把浏览器已保存的登录会话绑定到交互事务，推进状态机进入
// trust/consent 阶段，避免已有登录态的用户重复走 IDP 登录。
func (s *Service) AttachSession(ctx context.Context, interactionID string, session *Session) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		fresh, row, err := s.LoadInteraction(tx, interactionID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if fresh.Browser != session.Browser {
			return interactionError("本次授权不属于当前浏览器会话，请在发起连接的浏览器中继续，或返回应用重新发起连接。")
		}
		if row.State != "login" {
			return interactionError("本次授权已完成登录，请刷新页面继续。")
		}
		var a Account
		acRow, err := s.Store.Get(tx, "accounts", session.AccountID, &a, false)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		// 账号必须仍然处于活跃状态。
		if acRow.State != "active" {
			return interactionError("账号已停用，无法继续本次授权。")
		}
		fresh.AccountID, fresh.SessionID, fresh.AuthTime = session.AccountID, session.ID, time.Now().UTC()
		row.AccountID, row.State = session.AccountID, "authenticated"
		return s.Store.Put(tx, "interactions", interactionID, fresh, row)
	})
}

// LastGrantScopes 返回 (账号, 客户端, 资源) 下最近一条活跃授权的 scopes，
// 用于同意页预勾选上次选择；没有时返回 nil。
func (s *Service) LastGrantScopes(ctx context.Context, accountID string, c *Client, resource string) []string {
	g, _, err := s.reusableGrant(s.Store.Database().WithContext(ctx), accountID, c, resource)
	if err != nil {
		return nil
	}
	return g.Scopes
}

// ReusableGrant 校验 (账号, 客户端, 资源) 存在活跃授权且请求 scopes 未超出
// 上次授权，满足时返回该授权供“继续”按钮一键沿用。
func (s *Service) ReusableGrant(ctx context.Context, session *Session, c *Client, resource string, requested []string) (*Grant, error) {
	g, _, err := s.reusableGrant(s.Store.Database().WithContext(ctx), session.AccountID, c, resource)
	if err != nil {
		return nil, err
	}
	if !domain.Subset(requested, g.Scopes) {
		return nil, ErrNoReusableGrant
	}
	return g, nil
}

// reusableGrant 按 account_id + client_id 索引查询活跃授权，逐条打开载荷
// 匹配 resource 与客户端指纹，取最近一条未过期的记录。
func (s *Service) reusableGrant(db Database, accountID string, c *Client, resource string) (*Grant, persistence.Row, error) {
	var rows []persistence.Row
	if err := db.Table(persistence.Table("grants")).Where("account_id = ? AND client_id = ? AND state = ?", accountID, security.Hash(c.ID), "active").Order("updated_at DESC").Find(&rows).Error(); err != nil {
		return nil, persistence.Row{}, err
	}
	for _, row := range rows {
		if !row.ExpiresAt.After(time.Now()) {
			continue
		}
		var g Grant
		if err := s.Store.OpenRecord(row, &g, "grants"); err != nil {
			return nil, persistence.Row{}, err
		}
		// 指纹不匹配的旧授权不可沿用，防止客户端配置变更后被静默复用。
		if g.Resource != resource || g.Fingerprint != c.Fingerprint {
			continue
		}
		return &g, row, nil
	}
	return nil, persistence.Row{}, ErrNoReusableGrant
}

// ReuseGrant 在事务内完成一键沿用：绑定会话 → 按请求 scopes 重建授权并绑定
// 新会话 → 吊销旧授权 → 交互置 consented，随后回调端点可直接签发 code。
// 不直接复用旧授权的原因是 ActiveGrant 要求授权绑定的会话有效，旧授权的
// SessionID 在 refresh_token 过期场景下多半已死。
func (s *Service) ReuseGrant(ctx context.Context, interactionID string, session *Session, c *Client) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		fresh, row, err := s.LoadInteraction(tx, interactionID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if fresh.Browser != session.Browser {
			return interactionError("本次授权不属于当前浏览器会话，请在发起连接的浏览器中继续，或返回应用重新发起连接。")
		}
		if row.State != "login" {
			return interactionError("本次授权已完成登录，请刷新页面继续。")
		}
		// trust 校验前置：不受信的客户端即使有历史授权也必须先走 trust 页。
		trusted, err := s.Trusted(tx, session.AccountID, c)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if !trusted {
			return interactionError("你尚未确认信任此应用，请先完成应用信任确认。")
		}
		var srow persistence.Row
		if err := tx.Table(persistence.Table("sessions")).Where("id = ?", security.Hash(session.ID)).Lock().Take(&srow).Error(); err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if srow.State != "active" || !srow.ExpiresAt.After(time.Now()) {
			return interactionError("本次授权使用的登录会话已失效，请重新登录。")
		}
		old, _, err := s.reusableGrant(tx, session.AccountID, c, fresh.Resource)
		if err != nil {
			return err
		}
		if !domain.Subset(fresh.Request.Scopes, old.Scopes) {
			return ErrNoReusableGrant
		}
		scopes, err := domain.NormalizeScopes(fresh.Request.Scopes, domain.BusinessScopes())
		if err != nil {
			return err
		}
		// 绑定已有会话并按请求 scopes 创建新授权。
		fresh.AccountID, fresh.SessionID, fresh.AuthTime = session.AccountID, session.ID, time.Now().UTC()
		row.AccountID, row.State = session.AccountID, "authenticated"
		grantID := security.Random()
		g := Grant{ID: grantID, AccountID: fresh.AccountID, ClientID: c.ID, Resource: fresh.Resource, Scopes: scopes, SessionID: session.ID, Fingerprint: c.Fingerprint, AuthTime: fresh.AuthTime}
		exp := time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.GrantTTL))
		if exp.After(srow.ExpiresAt) {
			exp = srow.ExpiresAt
		}
		if err := s.Store.Create(tx, "grants", grantID, g, persistence.Row{GrantID: grantID, AccountID: g.AccountID, ClientID: security.Hash(c.ID), State: "active", ExpiresAt: exp}); err != nil {
			return err
		}
		// 吊销旧授权并级联吊销其令牌，保持状态链干净。
		if err := s.RevokeGrant(tx, old.ID); err != nil {
			return err
		}
		fresh.GrantID, fresh.DoneFlag, fresh.Request.Scopes = grantID, true, scopes
		row.State, row.GrantID = "consented", grantID
		return s.Store.Put(tx, "interactions", interactionID, fresh, row)
	})
}

// ResetInteraction 在切换账号时把交互事务退回 login 状态并清空账号绑定，
// 使自刷新后回到未登录视图。
func (s *Service) ResetInteraction(ctx context.Context, interactionID, browser string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		fresh, row, err := s.LoadInteraction(tx, interactionID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if fresh.Browser != browser {
			return interactionError("本次授权不属于当前浏览器会话，请在发起连接的浏览器中继续，或返回应用重新发起连接。")
		}
		if row.State == "consented" || row.State == "code" || row.State == "consumed" {
			return interactionError("本次授权已完成，无法切换账号。请返回应用查看连接结果。")
		}
		fresh.AccountID, fresh.SessionID, fresh.GrantID = "", "", ""
		fresh.AuthTime, fresh.TrustAfter, fresh.DoneFlag = time.Time{}, time.Time{}, false
		row.AccountID, row.State = "", "login"
		return s.Store.Put(tx, "interactions", interactionID, fresh, row)
	})
}
