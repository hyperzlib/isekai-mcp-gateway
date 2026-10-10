package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikiOrigins(t *testing.T) {
	for _, raw := range []string{"https://WIKI.example", "https://wiki.example:443/", "http://127.0.0.1:8080", "http://[::1]:80/"} {
		if _, err := ParseWikiOrigin(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "//wiki.example", "ftp://wiki.example", "https://user@wiki.example", "https://wiki.example/path", "https://wiki.example/?", "https://wiki.example/#", "https://wiki.example/?q=1", "https://wiki.example:", "https://wiki.example:0", "https://wiki.example:65536", "https://wiki.example/%2F"} {
		if _, err := ParseWikiOrigin(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	a, _ := ParseWikiOrigin("https://WIKI.example")
	b, _ := ParseWikiOrigin("https://wiki.example:443/")
	x, _ := HTTPOrigin(a)
	y, _ := HTTPOrigin(b)
	if x != y {
		t.Fatal("default port or casing differs")
	}
	c := Defaults()
	if err := json.Unmarshal([]byte(`{"mediawiki":{"apiUrl":"http://wiki.test/api.php"}}`), &c.App); err != nil {
		t.Fatal(err)
	}
	if c.App.MediaWiki.PublicURL != "" || len(c.App.MediaWiki.PublicOrigins) != 0 || c.validateWikiOrigins() != nil {
		t.Fatal("old configuration incompatible")
	}
	c.App.MediaWiki.PublicURL = "https://wiki.example"
	c.App.MediaWiki.PublicOrigins = []string{"https://alias.example"}
	if err := c.validateWikiOrigins(); err != nil {
		t.Fatal(err)
	}
	c.App.MediaWiki.PublicOrigins = []string{"https://alias.example/path"}
	if err := c.validateWikiOrigins(); err == nil {
		t.Fatal("invalid origin configuration accepted")
	}
	dir := t.TempDir()
	if err := GenerateSchemas(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "gateway-config.app.schema.json"))
	if err != nil || !strings.Contains(string(data), `"publicOrigins"`) || !strings.Contains(string(data), `"publicUrl"`) {
		t.Fatal("generated schema omits URL configuration", err)
	}
}
