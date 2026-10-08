package application

import (
	"context"
)

// Logout revokes only the grant bound to the authenticated request.
func (s *Service) Logout(ctx context.Context, p *Principal) error {
	if p == nil || p.GrantID == "" || p.AccountID == "" || p.OAuthClientID == "" {
		return ErrInvalidToken
	}
	return s.Store.Database().WithContext(ctx).Transaction(func(tx Database) error {
		g, _, err := s.ActiveGrant(tx, p.GrantID, true)
		if err != nil {
			return err
		}
		if g.AccountID != p.AccountID || g.ClientID != p.OAuthClientID {
			return ErrInvalidToken
		}
		return s.RevokeGrant(tx, g.ID)
	})
}
