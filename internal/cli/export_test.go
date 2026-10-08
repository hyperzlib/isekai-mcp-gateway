package gatewaycli

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assetbuiltin "isekai-mcp-gateway/assets"
	templatebuiltin "isekai-mcp-gateway/templates"
)

func TestExportsOfflineAndPreserveCustomizations(t *testing.T) {
	for _, tc := range []struct {
		command, dir, file string
		builtin            fs.FS
	}{
		{"export-templates", "templates", "text/help/index.md", templatebuiltin.FS()},
		{"export-assets", "assets", "auth/style.css", assetbuiltin.FS()},
	} {
		t.Run(tc.command, func(t *testing.T) {
			root := t.TempDir()
			run := func(overwrite bool) string {
				t.Helper()
				var out, stderr bytes.Buffer
				args := []string{"gateway", "--config-dir", filepath.Join(root, "absent"), "--data-dir", root, tc.command}
				if overwrite {
					args = append(args, "--overwrite")
				}
				if err := Run(context.Background(), args, &out, &stderr); err != nil {
					t.Fatal(err)
				}
				if stderr.Len() != 0 {
					t.Fatal(stderr.String())
				}
				return out.String()
			}
			if got := run(false); !strings.Contains(got, "skipped 0") {
				t.Fatal(got)
			}
			if err := fs.WalkDir(tc.builtin, ".", func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				want, err := fs.ReadFile(tc.builtin, name)
				if err != nil {
					return err
				}
				got, err := os.ReadFile(filepath.Join(root, tc.dir, filepath.FromSlash(name)))
				if err != nil {
					return err
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("export differs: %s", name)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(root, tc.dir, filepath.FromSlash(tc.file))
			if err := os.WriteFile(file, []byte("custom"), 0644); err != nil {
				t.Fatal(err)
			}
			if got := run(false); !strings.Contains(got, "Exported 0") {
				t.Fatal(got)
			}
			b, _ := os.ReadFile(file)
			if string(b) != "custom" {
				t.Fatal("customization overwritten")
			}
			run(true)
			b, _ = os.ReadFile(file)
			want, _ := fs.ReadFile(tc.builtin, tc.file)
			if !bytes.Equal(b, want) {
				t.Fatal("overwrite failed")
			}
			if _, err := os.Stat(filepath.Join(root, "config")); !os.IsNotExist(err) {
				t.Fatal("export created configuration")
			}
		})
	}
}
