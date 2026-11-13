package home

import (
	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
	"html"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/httpserver/middleware"
	"isekai-mcp-gateway/internal/templates"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestPublicGuideUsesConfiguredEndpointAndEscapesBrand(t *testing.T) {
	pages, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := &config.Config{}
	c.Static.Auth.Gateway.Resource = "https://wiki.example/mcp?x=\"<value>&y=1"
	s := &Service{Config: c, Pages: pages, Site: func() (string, string) { return "<script>alert(1)</script>", "" }}
	router := chi.NewRouter()
	s.Routes(router)
	req := httptest.NewRequest("GET", "http://attacker.invalid/", nil)
	w := httptest.NewRecorder()
	middleware.Headers(router).ServeHTTP(w, req)
	if w.Header().Get("Content-Security-Policy") != "" || w.Header().Get("X-Frame-Options") != "" {
		t.Fatal("public homepage must allow framing")
	}
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(html.UnescapeString(body), c.Static.Auth.Gateway.Resource) || strings.Contains(body, "attacker.invalid") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("invalid response: %d", w.Code)
	}
	match := regexp.MustCompile(`data-mcp-endpoint="([^"]*)"`).FindStringSubmatch(body)
	if len(match) != 2 || html.UnescapeString(match[1]) != c.Static.Auth.Gateway.Resource {
		t.Fatal("configured endpoint not escaped in data attribute")
	}
	for _, entry := range []string{"id=\"client-list\"", "id=\"guide-retry\"", "<noscript>", "/assets/home/guides/index.json", "vendor/markdown-it.min.js", "vendor/purify.min.js", "guide-renderer.js"} {
		if !strings.Contains(body, entry) {
			t.Fatalf("missing dynamic guide entry: %s", entry)
		}
	}
}
func TestCloudAddressWarning(t *testing.T) {
	for raw, want := range map[string]bool{"https://wiki.example/mcp": false, "http://wiki.example/mcp": true, "https://127.0.0.1/mcp": true, "https://[::1]/mcp": true, "https://192.168.1.2/mcp": true, "https://localhost/mcp": true} {
		if cloudWarning(raw) != want {
			t.Errorf("%s", raw)
		}
	}
}

func TestHomeLogoutCSRFToken(t *testing.T) {
	pages, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := &config.Config{}
	s := &Service{Config: c, Pages: pages, Site: func() (string, string) { return "Wiki", "" }, Login: func(http.ResponseWriter, *http.Request) (bool, string, string) { return true, "User", "" }}
	router := chi.NewRouter()
	s.Routes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "http://example.test/", nil))
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if w.Code != 200 || len(match) != 2 {
		t.Fatalf("home did not render CSRF token: status %d", w.Code)
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "gateway_csrf" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Path != "/auth" {
		t.Fatal("missing auth CSRF cookie")
	}
	handler := nosurf.New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	handler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Log(nosurf.Reason(r)); w.WriteHeader(400) }))
	handler.SetBaseCookie(http.Cookie{Name: "gateway_csrf", Path: "/auth", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	form := url.Values{"csrf_token": {html.UnescapeString(match[1])}}
	req := httptest.NewRequest("POST", "http://example.test/auth/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(cookie)
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	if result.Code != http.StatusNoContent {
		t.Fatalf("logout CSRF rejected: %d %s", result.Code, result.Body.String())
	}
}

func TestHomeHeaderLinkReload(t *testing.T) {
	pages, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var link *config.SiteHeaderLink
	s := &Service{Config: &config.Config{}, Pages: pages, Site: func() (string, string) { return "Wiki", "" }, HeaderLink: func() *config.SiteHeaderLink { return link }}
	render := func() string {
		w := httptest.NewRecorder()
		s.serve(w, httptest.NewRequest("GET", "https://example.test/", nil))
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
		return w.Body.String()
	}
	if strings.Contains(render(), "home-header-link") {
		t.Fatal("unexpected link")
	}
	link = &config.SiteHeaderLink{Label: "百科 <首页>", URL: "https://example.test/?a=1&b=2"}
	body := render()
	if !strings.Contains(body, "百科 &lt;首页&gt;") || !strings.Contains(body, "https://example.test/?a=1&amp;b=2") {
		t.Fatal("link missing or not escaped")
	}
	link = &config.SiteHeaderLink{Label: "新的链接", URL: "/wiki"}
	if body = render(); !strings.Contains(body, "新的链接") || !strings.Contains(body, `href="/wiki"`) {
		t.Fatal("updated link missing")
	}
}

func TestDiscoveryEndpoints(t *testing.T) {
	pages, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := &config.Config{}
	c.Static.Auth.Gateway.Resource = "https://example.test/mcp"
	c.Static.Server.PublicURL = "https://example.test/"
	s := &Service{Config: c, Pages: pages, Site: func() (string, string) { return "Example", "" }}
	router := chi.NewRouter()
	s.Routes(router)
	page := httptest.NewRecorder()
	router.ServeHTTP(page, httptest.NewRequest("GET", "/", nil))
	doc := httptest.NewRecorder()
	router.ServeHTTP(doc, httptest.NewRequest("GET", "/llms.txt", nil))
	if doc.Code != 200 || !strings.Contains(doc.Body.String(), c.Static.Auth.Gateway.Resource) || !strings.Contains(doc.Body.String(), "https://example.test/auth/apikeys") || !strings.Contains(page.Body.String(), "<!-- # Example MCP") || !strings.Contains(page.Body.String(), `href="/llms.txt"`) || page.Header().Get("Link") == "" {
		t.Fatal("discovery information missing")
	}
}
