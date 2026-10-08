package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/failure"
)

var ErrNoSession = failure.New("NO_SESSION", "登录已失效，请重新连接。", "auth.session", nil)

func (s *Service) LoadBrowserSession(ctx context.Context, sessionID, browser string) (*Session, error) {
	var session Session
	row, err := s.Store.Get(s.Store.Database().WithContext(ctx), "sessions", sessionID, &session, false)
	if err != nil {
		return nil, lookupError(err, ErrNoSession)
	}
	// 会话必须活跃、未过期且属于同一浏览器。
	if row.State != "active" || !row.ExpiresAt.After(time.Now()) || session.Browser != browser {
		return nil, ErrNoSession
	}
	var a Account
	ar, err := s.Store.Get(s.Store.Database().WithContext(ctx), "accounts", session.AccountID, &a, false)
	if err != nil {
		return nil, lookupError(err, ErrNoSession)
	}
	if ar.State != "active" {
		return nil, ErrNoSession
	}
	return &session, nil
}
