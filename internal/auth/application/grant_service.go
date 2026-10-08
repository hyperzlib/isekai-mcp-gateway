package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func (s *Service) ActiveGrant(db Database, id string, lock bool) (*Grant, persistence.Row, error) {
	var g Grant
	r, err := s.Store.Get(db, "grants", id, &g, lock)
	if err != nil {
		return nil, r, lookupError(err, domain.ErrInvalidGrant())
	}
	// 授权本身必须处于活跃状态且未过期。
	if r.State != "active" || !r.ExpiresAt.After(time.Now()) {
		return nil, r, domain.ErrInvalidGrant()
	}
	c, cr, err := s.LoadClient(db, g.ClientID, false)
	if err != nil {
		return nil, r, lookupError(err, domain.ErrInvalidGrant())
	}
	// 客户端必须有效且指纹未变，否则视为授权失效。
	if cr.State != "active" || c.Fingerprint != g.Fingerprint {
		return nil, r, domain.ErrInvalidGrant()
	}
	var account Account
	ar, err := s.Store.Get(db, "accounts", g.AccountID, &account, false)
	if err != nil {
		return nil, r, lookupError(err, domain.ErrInvalidGrant())
	}
	if ar.State != "active" {
		return nil, r, domain.ErrInvalidGrant()
	}
	var session Session
	sr, err := s.Store.Get(db, "sessions", g.SessionID, &session, false)
	if err != nil {
		return nil, r, lookupError(err, domain.ErrInvalidGrant())
	}
	// 会话必须有效、未过期且归属于同一账号。
	if sr.State != "active" || !sr.ExpiresAt.After(time.Now()) || session.AccountID != account.ID {
		return nil, r, domain.ErrInvalidGrant()
	}
	return &g, r, nil
}

func (s *Service) RevokeGrant(tx Database, id string) error {
	if err := tx.Table(persistence.Table("grants")).Where("id = ?", security.Hash(id)).Update("state", "revoked").Error(); err != nil {
		return err
	}
	// 级联吊销该授权下所有活跃的令牌与授权码。
	for _, kind := range []string{"access_tokens", "authorization_codes", "refresh_tokens"} {
		if err := tx.Table(persistence.Table(kind)).Where("grant_id = ? AND state = ?", id, "active").Update("state", "revoked").Error(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) TerminateSession(ctx context.Context, userID, clientID string) error {
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		var rows []persistence.Row
		if err := tx.Table(persistence.Table("grants")).Where("account_id = ? AND client_id = ?", userID, security.Hash(clientID)).Find(&rows).Error(); err != nil {
			return err
		}
		for _, r := range rows {
			if err := s.RevokeGrant(tx, r.GrantID); err != nil {
				return err
			}
		}
		return nil
	})
}
