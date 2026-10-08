package templates

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, "templates", filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// fixture 导出内嵌模板到临时目录，得到完整可用的模板树。
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	return root
}

func render(t *testing.T, m *Manager, name string, data any) string {
	t.Helper()
	var out bytes.Buffer
	if err := m.RenderHTML(&out, name, data); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestSourcesAndBuiltins(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"Title": "<script>", "Mode": "error", "Message": "&message", "SiteTitle": "", "SiteIconURL": "", "ClientName": "", "ClientLogo": "", "TrustLevel": ""}
	if got := render(t, m, "error", data); !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, "&amp;message") {
		t.Fatal(got)
	}
	var text bytes.Buffer
	if err := m.RenderText(&text, "help/index.md", nil); err != nil || !strings.Contains(text.String(), "wiki.help") {
		t.Fatal(err, text.String())
	}
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, "html/pages/error.html", `external {{.Title}}`)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := render(t, m, "error", data); got != "external &lt;script&gt;" {
		t.Fatal(got)
	}
	if err := os.Remove(filepath.Join(root, "templates/text/help/index.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root); err == nil {
		t.Fatal("incomplete directory accepted")
	}
	if err := m.Reload(); err == nil {
		t.Fatal("incomplete reload accepted")
	}
	if got := render(t, m, "error", data); got != "external &lt;script&gt;" {
		t.Fatal("failed reload replaced snapshot", got)
	}
	fileRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(fileRoot, "templates"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(fileRoot); err == nil {
		t.Fatal("regular file accepted as template directory")
	}
}

func TestLayoutsTextAndFailures(t *testing.T) {
	root := fixture(t)
	write(t, root, "html/layouts/test.html", `{{define "outer"}}[{{template "inner" .}}]{{end}}{{define "inner"}}{{block "body" .}}default{{end}}{{end}}`)
	write(t, root, "html/pages/test/a.html", `{{template "outer" .}}{{define "body"}}A{{.Value}}{{end}}`)
	write(t, root, "html/pages/test/b.html", `{{template "outer" .}}{{define "body"}}B{{.Value}}{{end}}`)
	write(t, root, "text/partials/rules.tmpl", `{{define "rules"}}Rules: {{.Value}}{{end}}`)
	write(t, root, "text/prompts/test.tmpl", `{{template "rules" .}} / {{.Value}}`)
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"Value": "<&>"}
	for name, want := range map[string]string{"test/a": "[A&lt;&amp;&gt;]", "test/b": "[B&lt;&amp;&gt;]"} {
		if got := render(t, m, name, data); got != want {
			t.Fatal(name, got)
		}
	}
	var out bytes.Buffer
	if err := m.RenderText(&out, "prompts/test.tmpl", data); err != nil || out.String() != "Rules: <&> / <&>" {
		t.Fatal(err, out.String())
	}
	out.Reset()
	if err := m.RenderHTML(&out, "test/a", map[string]any{}); err == nil || out.Len() != 0 {
		t.Fatal("missing variable emitted partial output", err, out.String())
	}
	if err := m.RenderText(&out, "prompts/test.tmpl", map[string]any{}); err == nil || out.Len() != 0 {
		t.Fatal("text missing variable accepted", err)
	}
	for _, invalid := range []string{`{{`, `{{template "missing" .}}`} {
		write(t, root, "html/pages/test/a.html", invalid)
		if err := m.Reload(); err == nil {
			t.Fatal("invalid reload accepted")
		}
		if got := render(t, m, "test/a", data); got != "[A&lt;&amp;&gt;]" {
			t.Fatal(got)
		}
	}
}

func TestWatchLifecycleAndConcurrentRendering(t *testing.T) {
	root := fixture(t)
	write(t, root, "html/partials/live.html", `{{define "live"}}one{{end}}`)
	write(t, root, "html/pages/live.html", `{{template "live" .}}`)
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				var out bytes.Buffer
				if err := m.RenderText(&out, "help/index.md", nil); err != nil {
					t.Error(err)
					return
				}
				out.Reset()
				if err := m.RenderHTML(&out, "error", map[string]any{"Title": "test", "Message": "test", "SiteTitle": "", "SiteIconURL": "", "ClientName": "", "ClientLogo": "", "TrustLevel": ""}); err != nil {
					t.Error(err)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}
	defer func() { cancel(); wg.Wait() }()
	wait := func(name, want string) {
		t.Helper()
		deadline := time.Now().Add(6 * time.Second)
		for time.Now().Before(deadline) {
			var out bytes.Buffer
			err := m.RenderHTML(&out, name, nil)
			if err == nil && out.String() == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("watch did not render %s as %q", name, want)
	}
	write(t, root, "html/partials/live.html", `{{define "live"}}two{{end}}`)
	wait("live", "two")
	// Replace the tree immediately, without waiting for an embedded snapshot.
	replacement := fixture(t)
	write(t, replacement, "html/partials/live.html", `{{define "live"}}replacement{{end}}`)
	write(t, replacement, "html/pages/live.html", `{{template "live" .}}`)
	if err := os.Rename(filepath.Join(root, "templates"), filepath.Join(root, "previous")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(replacement, "templates"), filepath.Join(root, "templates")); err != nil {
		t.Fatal(err)
	}
	wait("live", "replacement")
	write(t, root, "html/partials/live.html", `{{define "live"}}after replacement{{end}}`)
	wait("live", "after replacement")
	write(t, root, "html/pages/nested/new.html", `nested`)
	wait("nested/new", "nested")
	// 模拟编辑器原子替换：先写 .tmp 再重命名覆盖目标文件。
	path := filepath.Join(root, "templates/html/pages/nested/new.html")
	if err := os.WriteFile(path+".tmp", []byte("atomic"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
	wait("nested/new", "atomic")
	write(t, root, "html/pages/nested/new.html", `{{`)
	time.Sleep(400 * time.Millisecond)
	wait("nested/new", "atomic")
	// Move the entire custom tree away: the watcher must switch to embed.
	if err := os.Rename(filepath.Join(root, "templates"), filepath.Join(root, "saved")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		var out bytes.Buffer
		err := m.RenderHTML(&out, "live", nil)
		if err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var out bytes.Buffer
	if err := m.RenderHTML(&out, "live", nil); err == nil {
		t.Fatal("directory deletion did not switch to embed")
	}
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, "html/pages/live.html", `restored`)
	wait("live", "restored")
	cancel()
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWatchMissingAndEmbeddedDirectory(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, "html/pages/new.html", `appeared`)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		var out bytes.Buffer
		if err := m.RenderHTML(&out, "new", nil); err == nil && out.String() == "appeared" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("directory appearing from embedded mode was not loaded")
}
