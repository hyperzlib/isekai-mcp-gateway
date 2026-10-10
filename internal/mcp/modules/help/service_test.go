package help

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/templates"
)

func TestHelpUsingEmbeddedTextTemplates(t *testing.T) {
	m, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := New(m)
	for _, topic := range []string{"", "index", "connection", "authorization", "activity", "reading", "editing", "wikitext", "page-creation", "recovery", "moderation"} {
		got, err := s.Read(topic)
		if err != nil {
			t.Fatal(topic, err)
		}
		if got.Topic == "" || got.URI != "wiki://help/"+got.Topic || !strings.HasPrefix(got.Content, "#") || len(got.AvailableOperations) != 37 || !slices.Contains(got.AvailableOperations, "wiki.get_file_urls") {
			t.Fatalf("invalid result: %+v", got)
		}
	}
	// 扩展关闭时 moderation 返回未启用说明；启用数据契约下渲染完整正文。
	disabled, err := s.Read("moderation")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disabled.Content, "未启用待审编辑扩展") {
		t.Fatalf("disabled moderation help: %s", disabled.Content)
	}
	var buf bytes.Buffer
	if err := m.RenderText(&buf, "help/moderation.md", map[string]any{"ModerationEnabled": true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "null edit") {
		t.Fatalf("enabled moderation help: %s", buf.String())
	}
	if _, err := s.Read("../index"); err == nil {
		t.Fatal("invalid topic accepted")
	}
}
