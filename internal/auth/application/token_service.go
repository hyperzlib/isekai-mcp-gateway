package application

import (
	"context"
	"slices"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

func (s *Service) AccessByID(db Database, id, subject string) (*Token, *Grant, persistence.Row, error) {
	var t Token
	r, err := s.Store.Get(db, "access_tokens", id, &t, false)
	if err != nil {
		return nil, nil, r, lookupError(err, ErrInvalidToken)
	}
	// 令牌必须活跃、未过期且主体匹配。
	if r.State != "active" || !r.ExpiresAt.After(time.Now()) || t.Subject != subject {
		return nil, nil, r, ErrInvalidToken
	}
	g, _, err := s.ActiveGrant(db, t.GrantID, false)
	if err != nil {
		return nil, nil, r, lookupError(err, ErrInvalidToken)
	}
	// 令牌与授权的主体、客户端、资源必须一致。
	if g.AccountID != t.Subject || g.ClientID != t.ClientID || g.Resource != t.Resource {
		return nil, nil, r, ErrInvalidToken
	}
	return &t, g, r, nil
}

func (s *Service) ValidateToken(ctx context.Context, raw string) (*Principal, error) {
	// 限制令牌长度，避免超长输入消耗解密资源。
	if len(raw) > 16384 {
		return nil, ErrInvalidToken
	}
	plain, err := s.Decoder.Decrypt(raw)
	if err != nil {
		return nil, failure.New(failure.InvalidToken, ErrInvalidToken.Message, "auth.token.decode", err)
	}
	// 解密后的明文格式为 "令牌ID:主体"。
	id, sub, ok := strings.Cut(plain, ":")
	if !ok {
		return nil, ErrInvalidToken
	}
	t, g, row, err := s.AccessByID(s.Store.Database().WithContext(ctx), id, sub)
	if err != nil {
		return nil, lookupError(err, ErrInvalidToken)
	}
	// 令牌的资源必须与网关配置的资源一致。
	if t.Resource != s.Config.Static.Auth.Gateway.Resource {
		return nil, ErrInvalidToken
	}
	var account Account
	if _, err = s.Store.Get(s.Store.Database().WithContext(ctx), "accounts", g.AccountID, &account, false); err != nil {
		return nil, lookupError(err, ErrInvalidToken)
	}
	// 仅保留授权中仍然有效的 scope，防止授权收窄后仍携带旧权限。
	var scopes []string
	for _, v := range t.Scopes {
		if slices.Contains(g.Scopes, v) {
			scopes = append(scopes, v)
		}
	}
	var session Session
	if _, err = s.Store.Get(s.Store.Database().WithContext(ctx), "sessions", g.SessionID, &session, false); err != nil {
		return nil, lookupError(err, ErrInvalidToken)
	}
	// Wiki 用户 ID 无效时说明身份已不可用。
	if session.Identity.UserID <= 0 {
		return nil, ErrInvalidToken
	}
	return &Principal{Issuer: s.Config.Static.Auth.Gateway.Issuer, AccountID: account.ID, OAuthClientID: g.ClientID, GrantID: g.ID, WikiUserID: session.Identity.UserID, Scopes: scopes, ExpiresAt: row.ExpiresAt}, nil
}
