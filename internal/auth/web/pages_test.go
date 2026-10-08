package web

import (
	"bytes"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/templates"
)

func TestBuiltinAuthPages(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for mode, marker := range map[string]string{"login": "登录并继续", "trust": "信任此应用并继续", "consent": "同意并连接", "error": "重新发起连接", "grants": "撤销授权", "clients": "取消信任"} {
		var out bytes.Buffer
		data := pageData{Mode: mode, Title: "<title>", Message: "<message>", CSRF: "csrf-test", Interaction: "interaction", ClientName: "Test app", Username: "Test user", Countdown: 5, Scopes: []scopeView{{Name: "mcp:access", Label: "连接", Checked: true}}, Entries: []entryView{{ID: "entry", ClientName: "Test app"}}}
		if err := m.RenderHTML(&out, mode, data); err != nil {
			t.Fatal(mode, err)
		}
		body := out.String()
		for _, want := range []string{"<!doctype html>", "/assets/auth/style.css", "/assets/auth/auth.js", "&lt;title&gt;", marker} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q", mode, want)
			}
		}
		if mode != "error" && !strings.Contains(body, `value="csrf-test"`) {
			t.Fatal(mode, "missing CSRF field")
		}
	}
}
