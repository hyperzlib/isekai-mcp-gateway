package web

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/oidcclient"
	"isekai-mcp-gateway/internal/outbound"
)

func TestAuthorizationPagePolicies(t *testing.T) {
	manager := new(outbound.Manager)
	manager.Publish(&outbound.Bundle{Config: config.Defaults(), OidcClient: new(oidcclient.OidcConnect)})
	s := &Service{Service: &application.Service{Upstream: manager}, Pages: testPages{template.Must(template.New("page").Parse("{{.Mode}}"))}}
	for _, mode := range []string{"login", "trust", "consent", "clients", "grants"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.render(w, httptest.NewRequest("GET", "https://gateway.example/auth/login", nil), pageData{Mode: mode, Redirect: "http://127.0.0.1:4567/callback?state=private"})
			if w.Header().Get("Referrer-Policy") != "same-origin" {
				t.Fatal("HTML form must retain same-origin request identity")
			}
			csp := w.Header().Get("Content-Security-Policy")
			if !strings.Contains(csp, "form-action 'self' http://127.0.0.1:4567;") || strings.Contains(csp, "private") {
				t.Fatalf("callback origin must be allowed without its query: %s", csp)
			}
		})
	}
}

func TestLogoPageDataAndCSP(t *testing.T) {
	logo := "https://images.example:8443/a.png?q=private"
	other := "https://cdn.example/b.png"
	bad := "https://evil.example/; script-src *"
	s := &Service{Pages: testPages{template.Must(template.New("page").Parse("{{.LogoURL}} {{.TrustLevel}} {{range .Entries}}{{.TrustLevel}}{{end}}"))}}
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest("GET", "https://gateway.example/auth/clients", nil), pageData{Mode: "clients", LogoURL: &logo, TrustLevel: domain.TrustKnown, Entries: []entryView{{LogoURL: &logo, TrustLevel: domain.TrustUnknown}, {LogoURL: &other}, {LogoURL: &bad}}})
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "img-src 'self' https://images.example:8443 https://cdn.example;") || strings.Contains(csp, "private") || strings.Contains(csp, "evil") || strings.Count(csp, "images.example") != 1 {
		t.Fatalf("unexpected CSP: %s", csp)
	}
	if !strings.Contains(w.Body.String(), "known") || !strings.Contains(w.Body.String(), "unknown") || !strings.Contains(w.Body.String(), "images.example") {
		t.Fatal("presentation metadata missing from template data")
	}
}

func TestFormDestination(t *testing.T) {
	for raw, want := range map[string]string{
		"https://client.example/callback?q=secret":      "https://client.example",
		"com.example.app:/callback":                     "com.example.app:",
		"https://user:password@client.example/callback": "",
		"https://client.example/callback#fragment":      "",
		"javascript:alert(1)":                           "",
		"https://client.example/;form-action *":         "",
	} {
		if got := formDestination(raw); got != want {
			t.Errorf("formDestination(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSitePresentationAndReload(t *testing.T) {
	manager := new(outbound.Manager)
	defer config.Publish(nil)
	cfg := config.Defaults()
	cfg.App.Site = config.Site{Title: "<Site title>", IconURL: "https://images.example:8443/site.svg?private=1"}
	manager.Publish(&outbound.Bundle{Config: cfg, OidcClient: new(oidcclient.OidcConnect)})
	s := &Service{Service: &application.Service{Upstream: manager}, Pages: testPages{template.Must(template.New("page").Parse("{{.Title}}|{{.SiteTitle}}|{{.SiteIconURL}}|{{.Interaction}}"))}}
	logo := "https://images.example:8443/client.svg"
	for _, interaction := range []string{"", "oauth-interaction"} {
		w := httptest.NewRecorder()
		s.render(w, httptest.NewRequest("GET", "https://gateway.example/auth/login", nil), pageData{Title: "登录", Mode: "login", Interaction: interaction, LogoURL: &logo})
		if !strings.Contains(w.Body.String(), "登录|&lt;Site title&gt;|") || !strings.Contains(w.Body.String(), "site.svg?private=1") {
			t.Fatal("site data missing or title unescaped", w.Body.String())
		}
		csp := w.Header().Get("Content-Security-Policy")
		if strings.Count(csp, "https://images.example:8443") != 1 || strings.Contains(csp, "private") || strings.Contains(csp, "site.svg") {
			t.Fatal("icon CSP", csp)
		}
	}
	updated := config.Defaults()
	updated.App.Site = config.Site{Title: "Updated site", IconURL: "https://new-images.example/icon.svg"}
	manager.Publish(&outbound.Bundle{Config: updated, OidcClient: new(oidcclient.OidcConnect)})
	w := httptest.NewRecorder()
	s.render(w, httptest.NewRequest("GET", "https://gateway.example/auth/login", nil), pageData{Mode: "login"})
	if !strings.Contains(w.Body.String(), "Updated site") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "https://new-images.example") || strings.Contains(w.Header().Get("Content-Security-Policy"), "https://images.example") {
		t.Fatal("stale config snapshot")
	}
	updated = config.Defaults()
	updated.App.Site.IconURL = "/assets/icon.svg?v=2"
	manager.Publish(&outbound.Bundle{Config: updated, OidcClient: new(oidcclient.OidcConnect)})
	w = httptest.NewRecorder()
	s.render(w, httptest.NewRequest("GET", "https://gateway.example/auth/login", nil), pageData{Mode: "login"})
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "img-src 'self';") || !strings.Contains(w.Body.String(), "/assets/icon.svg?v=2") {
		t.Fatal("root path icon handling")
	}
}
