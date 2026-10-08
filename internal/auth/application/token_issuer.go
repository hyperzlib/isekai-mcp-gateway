package application

import (
	"context"
	"errors"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func (s *Service) CreateAccessToken(ctx context.Context, req domain.TokenRequest) (string, time.Time, error) {
	id, _, exp, err := s.issue(ctx, req, "", false)
	return id, exp, err
}

func (s *Service) CreateAccessAndRefreshTokens(ctx context.Context, req domain.TokenRequest, old string) (string, string, time.Time, error) {
	return s.issue(ctx, req, old, true)
}

func (s *Service) issue(ctx context.Context, req domain.TokenRequest, old string, refresh bool) (string, string, time.Time, error) {
	// 仅交互流程（授权码换令牌）与刷新令牌流程可签发令牌。
	var grantID string
	switch r := req.(type) {
	case *Interaction:
		grantID = r.GrantID
	case *Token:
		grantID = r.GrantID
	default:
		return "", "", time.Time{}, domain.ErrInvalidGrant()
	}
	id, next := security.Random(), ""
	if refresh {
		next = security.Random()
	}
	var expiry time.Time
	reused := false
	err := s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		g, gr, err := s.ActiveGrant(tx, grantID, true)
		if err != nil {
			return err
		}

		// 令牌必须归属同一客户端与资源，防止跨资源重放。
		info, _ := ctx.Value(RequestKey{}).(RequestInfo)
		if info.ClientID != g.ClientID || info.Resource != g.Resource {
			return domain.ErrInvalidGrant()
		}
		if !Subset(req.GetScopes(), g.Scopes) {
			return domain.ErrInvalidScope()
		}

		// 首次用授权码换取令牌：校验并消费授权码与交互。
		if old == "" {
			a := req.(*Interaction)
			var code Code
			codeRow, codeErr := s.Store.Get(tx, "authorization_codes", info.Code, &code, true)
			if codeErr != nil {
				return lookupError(codeErr, domain.ErrInvalidGrant())
			}
			if codeRow.State != "active" || !codeRow.ExpiresAt.After(time.Now()) || code.InteractionID != a.ID {
				return domain.ErrInvalidGrant()
			}
			fresh, row, err := s.LoadInteraction(tx, a.ID, true)
			if err != nil {
				return lookupError(err, domain.ErrInvalidGrant())
			}
			if row.State != "code" || fresh.GrantID != grantID {
				return domain.ErrInvalidGrant()
			}
			// The interaction is the common lock for every code issued from it.
			row.State = "consumed"
			if err = s.Store.Put(tx, "interactions", a.ID, fresh, row); err != nil {
				return err
			}
			if err = tx.Table(persistence.Table("authorization_codes")).Where("grant_id = ?", grantID).Update("state", "used").Error(); err != nil {
				return err
			}
		} else {
			// 刷新流程：检测刷新令牌重放，重放则吊销整个授权族。
			var t Token
			row, err := s.Store.Get(tx, "refresh_tokens", old, &t, true)
			if err != nil {
				return lookupError(err, domain.ErrInvalidGrant())
			}
			if t.GrantID != grantID {
				return domain.ErrInvalidGrant()
			}
			if row.State == "used" {
				reused = true
				return s.RevokeGrant(tx, grantID)
			}
			if row.State != "active" || !t.IdleExpiresAt.After(time.Now()) {
				return domain.ErrInvalidGrant()
			}
			if err = tx.Table(persistence.Table("refresh_tokens")).Where("id = ? AND state = ?", row.ID, "active").Update("state", "used").Error(); err != nil {
				return err
			}
		}
		// 访问令牌过期时间不超过授权的剩余有效期。
		now := time.Now().UTC()
		expiry = now.Add(Seconds(s.Config.Static.Auth.Gateway.AccessTTL))
		if expiry.After(gr.ExpiresAt) {
			expiry = gr.ExpiresAt
		}
		t := Token{ID: id, GrantID: grantID, Subject: g.AccountID, ClientID: g.ClientID, Resource: g.Resource, Scopes: req.GetScopes(), AuthTime: g.AuthTime}
		meta := persistence.Row{AccountID: g.AccountID, ClientID: security.Hash(g.ClientID), GrantID: grantID, State: "active", ExpiresAt: expiry}
		if err = s.Store.Create(tx, "access_tokens", id, t, meta); err != nil {
			return err
		}
		// 同时签发新的刷新令牌，其空闲过期时间不超过授权有效期。
		if refresh {
			t.ID = next
			t.IdleExpiresAt = now.Add(Seconds(s.Config.Static.Auth.Gateway.RefreshTTL))
			if t.IdleExpiresAt.After(gr.ExpiresAt) {
				t.IdleExpiresAt = gr.ExpiresAt
			}
			meta.ExpiresAt = gr.ExpiresAt
			if err = s.Store.Create(tx, "refresh_tokens", next, t, meta); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", "", time.Time{}, err
	}
	if reused {
		return "", "", time.Time{}, domain.ErrInvalidGrant()
	}
	return id, next, expiry, nil
}

func (s *Service) TokenRequestByRefreshToken(ctx context.Context, raw string) (*Token, error) {
	var t Token
	r, err := s.Store.Get(s.Store.Database().WithContext(ctx), "refresh_tokens", raw, &t, false)
	if err != nil {
		return nil, lookupError(err, domain.ErrInvalidGrant())
	}
	info, _ := ctx.Value(RequestKey{}).(RequestInfo)
	if info.ClientID != t.ClientID || info.Resource != t.Resource {
		return nil, domain.ErrInvalidGrant()
	}
	// 刷新令牌重放：吊销整个授权族并拒绝本次请求。
	if r.State == "used" {
		if err = s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error { return s.RevokeGrant(tx, t.GrantID) }); err != nil {
			return nil, err
		}
		return nil, domain.ErrInvalidGrant()
	}
	if r.State != "active" || !t.IdleExpiresAt.After(time.Now()) {
		return nil, domain.ErrInvalidGrant()
	}
	if _, _, err = s.ActiveGrant(s.Store.Database().WithContext(ctx), t.GrantID, false); err != nil {
		return nil, err
	}
	// 请求携带 scope 时按规则收窄基本 scope，自定义权限保持继承。
	if info.RefreshScopesPresent {
		t.Scopes, err = domain.RefreshScopes(t.Scopes, info.RefreshScopes)
		if err != nil {
			return nil, domain.ErrInvalidScope()
		}
	}
	return &t, nil
}

func (s *Service) GetRefreshTokenInfo(ctx context.Context, clientID, raw string) (string, string, error) {
	var t Token
	_, err := s.Store.Get(s.Store.Database().WithContext(ctx), "refresh_tokens", raw, &t, false)
	if err != nil {
		return "", "", lookupError(err, domain.ErrInvalidRefreshToken)
	}
	if t.ClientID != clientID {
		return "", "", domain.ErrInvalidRefreshToken
	}
	return t.Subject, raw, nil
}

func (s *Service) RevokeToken(ctx context.Context, token, subject, clientID string) error {
	var t Token
	// 先按访问令牌查找，未命中再按刷新令牌查找。
	_, err := s.Store.Get(s.Store.Database().WithContext(ctx), "access_tokens", token, &t, false)
	if errors.Is(err, ErrRecordNotFound) {
		_, err = s.Store.Get(s.Store.Database().WithContext(ctx), "refresh_tokens", token, &t, false)
	}
	if errors.Is(err, ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return failure.Wrap("auth.revoke", err)
	}
	if t.ClientID != clientID {
		return nil
	}
	if err = s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error { return s.RevokeGrant(tx, t.GrantID) }); err != nil {
		return failure.Wrap("auth.revoke", err)
	}
	return nil
}
