package mediawiki

import (
	"context"
	"isekai-mcp-gateway/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This verifies public discovery against the configured local MediaWiki. It
// never logs credentials or performs a write.
func TestLocalWikiCapabilities(t *testing.T) {
	dir := os.Getenv("GATEWAY_WIKI_TEST_CONFIG")
	if dir == "" {
		t.Skip("set GATEWAY_WIKI_TEST_CONFIG to verify local MediaWiki capabilities")
	}
	cfg := config.Defaults()
	if e := config.Decode(filepath.Join(dir, "server.json"), &cfg.Static); e != nil {
		t.Fatal(e)
	}
	if e := config.Decode(filepath.Join(dir, "app.json"), &cfg.App); e != nil {
		t.Fatal(e)
	}
	client := New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := client.Probe(ctx); e != nil {
		t.Fatal(e)
	}
	if e := client.Capabilities(ctx, cfg); e != nil {
		t.Fatal(e)
	}
}
