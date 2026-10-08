package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func trustKey(account, client string) string { return account + "\x00" + client }

func (s *Service) Trusted(tx Database, account string, c *Client) (bool, error) {
	// 预注册（trusted）客户端始终受信；已知与动态注册客户端需账号首次
	// 确认后留下的信任记录，并核对指纹。
	level := c.TrustLevel
	if level == "" {
		level = domain.DefaultTrust(c.Source)
	}
	if level == domain.TrustTrusted {
		return true, nil
	}
	var trust ClientTrust
	_, err := s.Store.Get(tx, "client_trusts", trustKey(account, c.ID), &trust, false)
	if errors.Is(err, ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return trust.Fingerprint == c.Fingerprint, nil
}

func (s *Service) ConfirmTrust(ctx context.Context, a *Interaction, c *Client) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		client, cr, err := s.LoadClient(tx, c.ID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		// 交互记录的客户端指纹必须与当前客户端一致。
		if client.Fingerprint != a.ClientFingerprint {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		fresh, ar, err := s.LoadInteraction(tx, a.ID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		// 必须已完成认证，且信任倒计时已结束。
		if ar.State != "authenticated" || fresh.AccountID != a.AccountID || fresh.TrustAfter.IsZero() || time.Now().Before(fresh.TrustAfter) {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		// 首次信任时把 pending 客户端提升为 active。
		if cr.State == "pending" {
			if err = s.PersistClient(tx, client, "active", time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		trust := ClientTrust{AccountID: a.AccountID, ClientID: c.ID, Fingerprint: c.Fingerprint, CreatedAt: now, LastConfirmedAt: now}
		var previous ClientTrust
		// 重新确认信任时保留首次创建时间。
		_, e := s.Store.Get(tx, "client_trusts", trustKey(a.AccountID, c.ID), &previous, true)
		if e == nil {
			trust.CreatedAt = previous.CreatedAt
		} else if !errors.Is(e, ErrRecordNotFound) {
			return e
		}
		return s.Store.Put(tx, "client_trusts", trustKey(a.AccountID, c.ID), trust, persistence.Row{AccountID: a.AccountID, ClientID: security.Hash(c.ID), State: "active", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)})
	})
}

func (s *Service) Consent(ctx context.Context, a *Interaction, c *Client, requested []string) error {
	scopes, err := NormalizeScopes(requested, BusinessScopes())
	if err != nil {
		return err
	}
	// 请求的 scope 必须属于可授予集合，且不超过客户端注册范围。
	if !Subset(scopes, ConsentScopes(a.Request.Scopes, BusinessScopes())) || !RequestedScopesAllowed(scopes, strings.Fields(c.Scope)) {
		return domain.ErrInvalidScope()
	}
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		client, cr, err := s.LoadClient(tx, c.ID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if cr.State != "active" || client.Fingerprint != a.ClientFingerprint {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		fresh, ar, err := s.LoadInteraction(tx, a.ID, true)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if ar.State != "authenticated" || fresh.AccountID != a.AccountID {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		// 客户端必须已被该账号信任。
		Trusted, err := s.Trusted(tx, a.AccountID, client)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if !Trusted {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		// 会话必须仍然有效。
		var session Session
		sr, err := s.Store.Get(tx, "sessions", fresh.SessionID, &session, false)
		if err != nil {
			return lookupError(err, domain.ErrInvalidGrant())
		}
		if sr.State != "active" || !sr.ExpiresAt.After(time.Now()) {
			return &FlowError{Code: "INVALID_INTERACTION", Message: "这次连接已失效或尚未准备完成，请重新发起。", Status: 400}
		}
		grantID := security.Random()
		g := Grant{ID: grantID, AccountID: fresh.AccountID, ClientID: c.ID, Resource: fresh.Resource, Scopes: scopes, SessionID: fresh.SessionID, Fingerprint: c.Fingerprint, AuthTime: fresh.AuthTime}
		// 授权有效期不超过会话有效期。
		exp := time.Now().UTC().Add(Seconds(s.Config.Static.Auth.Gateway.GrantTTL))
		if exp.After(sr.ExpiresAt) {
			exp = sr.ExpiresAt
		}
		if err = s.Store.Create(tx, "grants", grantID, g, persistence.Row{GrantID: grantID, AccountID: g.AccountID, ClientID: security.Hash(c.ID), State: "active", ExpiresAt: exp}); err != nil {
			return err
		}
		fresh.GrantID, fresh.DoneFlag, fresh.Request.Scopes = grantID, true, scopes
		ar.State, ar.GrantID = "consented", grantID
		return s.Store.Put(tx, "interactions", a.ID, fresh, ar)
	})
}
