package home

import (
	"github.com/go-chi/chi/v5"
	"html"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/httpserver/middleware"
	"isekai-mcp-gateway/internal/templates"
	"net/http/httptest"
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
