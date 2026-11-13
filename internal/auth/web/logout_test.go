package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
)

func TestLogoutRedirectsHome(t *testing.T) {
	s := &Service{Service: &application.Service{Config: &config.Config{}}}
	form := url.Values{"returnTo": {"/auth/apikeys"}}
	req := httptest.NewRequest(http.MethodPost, "https://example.test/auth/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Logout(w, req)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("logout redirect: %d %q", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "gateway_session" || cookies[0].MaxAge >= 0 {
		t.Fatalf("session cookie not cleared: %v", cookies)
	}
}
