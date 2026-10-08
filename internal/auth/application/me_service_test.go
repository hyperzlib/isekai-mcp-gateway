package application

import (
	"context"
	"testing"

	"isekai-mcp-gateway/internal/persistence"
)

type meStore struct {
	*logoutStore
	disabled string
}

func (s *meStore) Get(db Database, kind, key string, value any, lock bool) (persistence.Row, error) {
	r, err := s.logoutStore.Get(db, kind, key, value, lock)
	if kind == "accounts" {
		*value.(*Account) = Account{ID: "account", WikiUserID: 123, Username: "Example", DisplayName: "示例用户", Subject: "private-subject"}
	}
	if kind == s.disabled {
		r.State = "revoked"
	}
	return r, err
}

func TestCurrentUserAuthorization(t *testing.T) {
	// 账号或授权被停用时 /me 必须拒绝。
	for _, disabled := range []string{"", "accounts", "grants"} {
		store := &meStore{logoutStore: &logoutStore{db: new(logoutDB), grant: Grant{ID: "grant", AccountID: "account", ClientID: "client", Fingerprint: "fingerprint"}}, disabled: disabled}
		s := &Service{Store: store}
		user, err := s.CurrentUser(context.Background(), &Principal{AccountID: "account", OAuthClientID: "client", GrantID: "grant", Scopes: []string{"mcp:access"}})
		if disabled == "" {
			if err != nil || user.UserID != 123 || user.Username != "Example" {
				t.Fatalf("user without profile: %+v, %v", user, err)
			}
		} else if err == nil {
			t.Fatalf("accepted disabled %s", disabled)
		}
	}
}
