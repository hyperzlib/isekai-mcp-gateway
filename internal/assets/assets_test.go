package assets

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestGuideResourcesAndLiveExternalUpdates(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	var clients []struct{ ID, Name, Icon, Guide string }
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/assets/home/guides/index.json", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" || json.Unmarshal(w.Body.Bytes(), &clients) != nil || len(clients) != 9 {
		t.Fatalf("invalid guide index: %d %s", w.Code, w.Body.String())
	}
	paths := []string{"/assets/home/vendor/markdown-it.min.js", "/assets/home/vendor/markdown-it-container.min.js", "/assets/home/vendor/purify.min.js", "/assets/home/guide-renderer.js"}
	for _, client := range clients {
		paths = append(paths, "/assets/home/guides/"+client.Guide, client.Icon)
	}
	for _, path := range paths {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("missing guide resource: %s %d", path, w.Code)
		}
	}
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"# First revision", "# Updated revision"} {
		if err := os.WriteFile(filepath.Join(root, "assets/home/guides/codex.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/assets/home/guides/codex.md", nil))
		if w.Code != 200 || w.Body.String() != content || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
			t.Fatalf("stale or incorrect guide: %v %s", w.Header(), w.Body.String())
		}
	}
}

func TestEmbeddedAndExternalResources(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	s.Routes(router)
	for _, path := range []string{"/assets/auth/auth.js", "/assets/auth/style.css"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatal(path, w.Code)
		}
	}
	// 目录列举、Go 源码与路径穿越都必须被拒绝。
	for _, path := range []string{"/assets/auth/", "/assets/embed.go", "/assets/../go.mod"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	if _, err := Export(root, false); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"/* custom one */", "/* custom two */"} {
		if err := os.WriteFile(filepath.Join(root, "assets/auth/style.css"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/assets/auth/style.css", nil))
		if w.Code != 200 || w.Body.String() != content {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if err := os.Remove(filepath.Join(root, "assets/auth/auth.js")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root); err == nil {
		t.Fatal("incomplete assets accepted")
	}
}
