package application

import (
	"context"
	"slices"
	"testing"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
)

type consentStore struct {
	*logoutStore
	interaction Interaction
	issued      Grant
}

func (s *consentStore) Get(db Database, kind, key string, value any, lock bool) (persistence.Row, error) {
	r, err := s.logoutStore.Get(db, kind, key, value, lock)
	if kind == "interactions" {
		*value.(*Interaction) = s.interaction
		r.State = "authenticated"
	}
	return r, err
}
func (s *consentStore) Create(_ Database, kind, _ string, value any, _ persistence.Row) error {
	if kind == "grants" {
		s.issued = value.(Grant)
	}
	return nil
}
func (s *consentStore) Put(_ Database, _, _ string, _ any, _ persistence.Row) error { return nil }

func TestConsentGrantsBusinessScopes(t *testing.T) {
	// 业务 scope 固定注册；基础 scope 需在请求范围内。
	a := Interaction{ID: "interaction", AccountID: "account", SessionID: "session", ClientFingerprint: "fingerprint"}
	a.Request.Scopes = []string{"mcp:access"}
	store := &consentStore{logoutStore: &logoutStore{db: new(logoutDB)}, interaction: a}
	cfg := config.Defaults()
	s := &Service{Store: store, Config: cfg}
	c := &Client{ID: "client", Scope: "mcp:access"}
	err := s.Consent(context.Background(), &a, c, []string{"wiki:edit", "wiki:upload"})
	if err != nil || !slices.Contains(store.issued.Scopes, "wiki:edit") || !slices.Contains(store.issued.Scopes, "wiki:read") || !slices.Contains(store.issued.Scopes, "wiki:upload") {
		t.Fatalf("business consent: %+v %v", store.issued, err)
	}
	if err := s.Consent(context.Background(), &a, c, []string{"unknown:scope"}); err == nil {
		t.Fatal("unconfigured custom scope accepted")
	}
	if err := s.Consent(context.Background(), &a, c, []string{"email"}); err == nil {
		t.Fatal("unrequested basic scope accepted")
	}
}
