package web

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
)

type adminDB struct{ application.Database }

func (d *adminDB) WithContext(context.Context) application.Database { return d }

type adminStore struct {
	application.RecordStore
	account      application.Account
	browser      string
	failAccount  bool
	accountReads int
}

func (s *adminStore) Database() application.Database { return &adminDB{} }
func (s *adminStore) Get(_ application.Database, kind, _ string, v any, _ bool) (persistence.Row, error) {
	if kind == "sessions" {
		*v.(*application.Session) = application.Session{AccountID: "account", Browser: s.browser}
	} else if kind == "accounts" {
		s.accountReads++
		if s.failAccount && s.accountReads == 2 {
			return persistence.Row{}, errors.New("database failure")
		}
		*v.(*application.Account) = s.account
	}
	return persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestRequireAdmin(t *testing.T) {
	for _, tc := range []struct {
		name, issuer, subject string
		loggedIn, fail        bool
		status                int
	}{
		{"admin", "https://id.example", "second", true, false, 204},
		{"other subject", "https://id.example", "other", true, false, 403},
		{"issuer mismatch", "https://other.example", "second", true, false, 403},
		{"missing session", "https://id.example", "second", false, false, 303},
		{"storage failure", "https://id.example", "second", true, true, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Static.Admin.Subs = []string{"first", "second"}
			cfg.App.Auth.OidcClient.Issuer = "https://id.example"
			manager := new(outbound.Manager)
			manager.Publish(&outbound.Bundle{Config: cfg})
			defer config.Publish(nil)
			store := &adminStore{account: application.Account{ID: "account", Issuer: tc.issuer, Subject: tc.subject}, failAccount: tc.fail}
			s := &Service{Service: &application.Service{Config: cfg, Store: store, Upstream: manager}, Pages: testPages{template.Must(template.New("error").Parse("{{.Message}}"))}}
			req := httptest.NewRequest("GET", "https://gateway.example/auth/admin", nil)
			if tc.loggedIn {
				initial := httptest.NewRecorder()
				store.browser = s.browser(initial, req)
				for _, c := range initial.Result().Cookies() {
					req.AddCookie(c)
				}
				req.AddCookie(&http.Cookie{Name: "gateway_session", Value: "session"})
			}
			out := httptest.NewRecorder()
			s.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(out, req)
			if out.Code != tc.status {
				t.Fatalf("status = %d, want %d", out.Code, tc.status)
			}
			if tc.status == 303 && out.Header().Get("Location") != "/auth/login" {
				t.Fatal("missing login redirect")
			}
		})
	}
}
