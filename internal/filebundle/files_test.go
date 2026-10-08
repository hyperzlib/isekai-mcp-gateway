package filebundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestExport(t *testing.T) {
	source := fstest.MapFS{"nested/a.txt": {Data: []byte("builtin")}}
	root := filepath.Join(t.TempDir(), "export")
	r, err := Export(source, root, false)
	if err != nil || r.Written != 1 {
		t.Fatal(r, err)
	}
	file := filepath.Join(root, "nested/a.txt")
	if err := os.WriteFile(file, []byte("custom"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err = Export(source, root, false)
	if err != nil || r.Skipped != 1 {
		t.Fatal(r, err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "custom" {
		t.Fatal(string(b))
	}
	r, err = Export(source, root, true)
	if err != nil || r.Written != 1 {
		t.Fatal(r, err)
	}
	b, _ = os.ReadFile(file)
	if string(b) != "builtin" {
		t.Fatal(string(b))
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err = Export(source, root, false); err == nil {
		t.Fatal("directory conflict accepted")
	}
	// 祖先路径是普通文件时，导出应失败而不是覆盖它。
	ancestor := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(ancestor, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Export(source, filepath.Join(ancestor, "export"), false); err == nil {
		t.Fatal("file ancestry conflict accepted")
	}
}

func TestExportRejectsSymlinks(t *testing.T) {
	outside := t.TempDir()
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	source := fstest.MapFS{"a.txt": {Data: []byte("builtin")}}
	if _, err := Export(source, filepath.Join(link, "export"), true); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if err := os.WriteFile(filepath.Join(outside, "target"), []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(source, root, true); err == nil {
		t.Fatal("symlink file accepted")
	}
	b, _ := os.ReadFile(filepath.Join(outside, "target"))
	if string(b) != "safe" {
		t.Fatal("symlink target modified")
	}
	if _, err := os.Stat(filepath.Join(outside, "export")); !os.IsNotExist(err) {
		t.Fatal("symlink export wrote outside", err)
	}
}

func TestSelectFallbackOnlyForAbsentDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	builtin := fstest.MapFS{"a": {Data: []byte("builtin")}}
	files, err := Select(filepath.Join(root, "missing"), builtin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(files, "a")
	if err != nil || string(b) != "builtin" {
		t.Fatal(err)
	}
	if _, err := Select(filepath.Join(root, "file"), builtin); err == nil {
		t.Fatal("file accepted")
	}
}
