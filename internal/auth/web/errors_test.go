package web

import (
	"errors"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

func TestBrowserErrorIsControlledAndCorrelated(t *testing.T) {
	s := &Service{Pages: testPages{template.Must(template.New("page").Parse("{{.Message}}"))}}
	r := httptest.NewRequest("GET", "/auth/oidc_client/callback", nil)
	r = r.WithContext(failure.NewRequest(r.Context()))
	w := httptest.NewRecorder()
	cause := errors.New("secret https://private SQL internal path")
	s.flowError(w, r, &application.FlowError{Message: "无法完成登录。", Status: 500, Cause: failure.Wrap("login", cause)})
	if w.Code != 500 || !strings.Contains(w.Body.String(), failure.RequestID(r.Context())) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("unsafe browser error: %d %s", w.Code, w.Body.String())
	}
}
