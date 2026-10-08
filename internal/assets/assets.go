// Package assets serves static resources independently of template rendering.
package assets

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	builtin "isekai-mcp-gateway/assets"
	"isekai-mcp-gateway/internal/filebundle"
)

type Service struct{ files fs.FS }

func New(dataDir string) (*Service, error) {
	root, err := filepath.Abs(filepath.Join(dataDir, "assets"))
	if err != nil {
		return nil, err
	}
	files, err := filebundle.Select(root, builtin.FS())
	if err != nil {
		return nil, err
	}
	if err = filebundle.Validate(files, builtin.FS(), root); err != nil {
		return nil, err
	}
	return &Service{files: files}, nil
}

func (s *Service) Routes(r chi.Router) { r.Handle("/assets/*", s.Handler()) }

func (s *Service) Handler() http.Handler {
	// Do not expose directory listings or Go files in the source resource tree.
	server := http.StripPrefix("/assets/", http.FileServer(http.FS(s.files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅处理 /assets/ 前缀，其余一律 404。
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		name := r.URL.Path[len("/assets/"):]
		// 拒绝非法路径与源码文件，防止目录列举或信息泄漏。
		if !fs.ValidPath(name) || filepath.Ext(name) == ".go" {
			http.NotFound(w, r)
			return
		}
		// 只允许访问存在的普通文件，目录与特殊文件返回 404。
		info, err := fs.Stat(s.files, name)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		server.ServeHTTP(w, r)
	})
}

func Export(dataDir string, overwrite bool) (filebundle.ExportResult, error) {
	return filebundle.Export(builtin.FS(), filepath.Join(dataDir, "assets"), overwrite)
}
