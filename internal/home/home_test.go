package home

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"html"
	"isekai-mcp-gateway/internal/config"
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
	c.Static.Auth.Gateway.Resource = "https://wiki.example/mcp"
	s := &Service{Config: c, Pages: pages, Site: func() (string, string) { return "<script>alert(1)</script>", "" }}
	router := chi.NewRouter()
	s.Routes(router)
	req := httptest.NewRequest("GET", "http://attacker.invalid/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, c.Static.Auth.Gateway.Resource) || strings.Contains(body, "attacker.invalid") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("invalid response: %d", w.Code)
	}
	for _, match := range regexp.MustCompile(`(?s)<pre><code[^>]*>(.*?)</code></pre>`).FindAllStringSubmatch(body, -1) {
		snippet := html.UnescapeString(match[1])
		if strings.HasPrefix(snippet, "{") && !json.Valid([]byte(snippet)) {
			t.Fatalf("invalid rendered JSON: %s", snippet)
		}
	}
	if strings.Count(body, "class=\"home-guide\"") != 9 {
		t.Fatal("missing guides")
	}
}
func TestCloudAddressWarning(t *testing.T) {
	for raw, want := range map[string]bool{"https://wiki.example/mcp": false, "http://wiki.example/mcp": true, "https://127.0.0.1/mcp": true, "https://[::1]/mcp": true, "https://192.168.1.2/mcp": true, "https://localhost/mcp": true} {
		if cloudWarning(raw) != want {
			t.Errorf("%s", raw)
		}
	}
}
