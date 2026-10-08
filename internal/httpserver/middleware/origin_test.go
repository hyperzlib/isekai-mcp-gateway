package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justinas/nosurf"

	"isekai-mcp-gateway/internal/config"
)

func TestOAuthNavigationAndMCPOrigins(t *testing.T) {
	c := config.Defaults()
	c.Static.Server.PublicURL = "https://gateway.example"
	next := Origin(c, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, test := range []struct {
		method, path, origin string
		status               int
	}{
		{"GET", "/auth/interaction/test", "null", 204},
		{"GET", "/auth/oidc_client/callback", "https://idp.example", 204},
		{"GET", "/oauth/authorize", "null", 204},
		{"GET", "/oauth/authorize/callback", "null", 204},
		{"POST", "/auth/interaction/test", "null", 403},
		{"POST", "/auth/login", "https://attacker.example", 403},
		{"GET", "/mcp", "null", 403},
		{"POST", "/mcp", "https://attacker.example", 403},
		{"POST", "/mcp", "https://gateway.example", 204},
		{"OPTIONS", "/mcp", "https://gateway.example", 204},
		{"GET", "/oauth/userinfo", "null", 403},
	} {
		r := httptest.NewRequest(test.method, "https://gateway.example"+test.path, nil)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		next.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Errorf("%s %s Origin=%s: got %d, want %d", test.method, test.path, test.origin, w.Code, test.status)
		}
	}
}

func TestFormOriginAndCSRFAreBothRequired(t *testing.T) {
	c := config.Defaults()
	c.Static.Server.PublicURL = "https://gateway.example"
	csrf := nosurf.New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = io.WriteString(w, nosurf.Token(r))
			return
		}
		w.WriteHeader(204)
	}))
	csrf.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "CSRF validation failed", http.StatusForbidden)
	}))
	handler := Origin(c, csrf)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "https://gateway.example/auth/login", nil))
	cookies, token := w.Result().Cookies(), w.Body.String()
	for _, test := range []struct {
		name, origin, token string
		status              int
	}{
		{"valid", c.Static.Server.PublicURL, token, 204},
		{"foreign with valid token", "https://attacker.example", token, 403},
		{"null with valid token", "null", token, 403},
		{"missing token", c.Static.Server.PublicURL, "", 403},
		{"forged token", c.Static.Server.PublicURL, "forged", 403},
		{"missing origin and referer", "", token, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "https://gateway.example/auth/login", strings.NewReader(url.Values{"csrf_token": {test.token}}.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Origin", test.origin)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d, want %d", w.Code, test.status)
			}
		})
	}
}
