package application

import (
	"context"
	"slices"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/persistence"
)

type refreshScopeStore struct {
	*logoutStore
	token Token
}

func (s *refreshScopeStore) Get(db Database, kind, key string, value any, lock bool) (persistence.Row, error) {
	if kind == "refresh_tokens" {
		*value.(*Token) = s.token
		return persistence.Row{State: "active"}, nil
	}
	return s.logoutStore.Get(db, kind, key, value, lock)
}

func TestRefreshLoadsGrantedScopes(t *testing.T) {
	// 未携带 scope 时沿用原权限；携带时仅收窄基础权限。
	prior := []string{"openid", "offline_access", "profile", "mcp:access", "wiki:edit", "future:custom"}
	store := &refreshScopeStore{logoutStore: &logoutStore{db: new(logoutDB), grant: Grant{ID: "grant", AccountID: "account", ClientID: "client", Fingerprint: "fingerprint"}}, token: Token{GrantID: "grant", ClientID: "client", Resource: "resource", Scopes: prior, IdleExpiresAt: time.Now().Add(time.Hour)}}
	s := &Service{Store: store}
	for _, present := range []bool{false, true} {
		ctx := context.WithValue(context.Background(), RequestKey{}, RequestInfo{ClientID: "client", Resource: "resource", RefreshScopesPresent: present, RefreshScopes: []string{"openid"}})
		token, err := s.TokenRequestByRefreshToken(ctx, "refresh")
		want := prior
		if present {
			want = []string{"openid", "mcp:access", "wiki:edit", "future:custom"}
		}
		if err != nil || !slices.Equal(token.Scopes, want) {
			t.Fatalf("refresh: %v %v", token, err)
		}
	}
}
