package application

import (
	"context"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
)

func (s *Service) CurrentUser(ctx context.Context, p *Principal) (domain.UserInfo, error) {
	if p == nil {
		return domain.UserInfo{}, ErrInvalidToken
	}
	db := s.Store.Database().WithContext(ctx)
	g, _, err := s.ActiveGrant(db, p.GrantID, false)
	if err != nil {
		return domain.UserInfo{}, lookupError(err, ErrInvalidToken)
	}
	// 授权主体与客户端必须与当前身份一致。
	if g.AccountID != p.AccountID || g.ClientID != p.OAuthClientID {
		return domain.UserInfo{}, ErrInvalidToken
	}
	var account Account
	row, err := s.Store.Get(db, "accounts", p.AccountID, &account, false)
	if err != nil {
		return domain.UserInfo{}, lookupError(err, failure.New("USER_INFORMATION_UNAVAILABLE", "用户信息不可用。", "auth.me", nil))
	}
	if row.State != "active" || account.ID != p.AccountID {
		return domain.UserInfo{}, failure.New("USER_INFORMATION_UNAVAILABLE", "用户信息不可用。", "auth.me", nil)
	}
	return domain.UserInfo{UserID: account.WikiUserID, Username: account.Username, DisplayName: account.DisplayName}, nil
}
