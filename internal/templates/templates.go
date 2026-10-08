// Package templates renders immutable HTML and text template snapshots.
package templates

import (
	"bytes"
	"context"
	"fmt"
	html "html/template"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	text "text/template"
	"text/template/parse"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/filebundle"
	builtin "isekai-mcp-gateway/templates"
)

type HTMLRenderer interface {
	RenderHTML(io.Writer, string, any) error
}
type TextRenderer interface {
	RenderText(io.Writer, string, any) error
}

type snapshot struct {
	pages map[string]*html.Template
	text  *text.Template
}

type Manager struct {
	root     string
	current  atomic.Pointer[snapshot]
	reloadMu sync.Mutex
	watchMu  sync.Mutex
	watch    *watchRun
}

// New loads and validates templates without starting a watcher.
func New(dataDir string) (*Manager, error) {
	root, err := filepath.Abs(filepath.Join(dataDir, "templates"))
	if err != nil {
		return nil, err
	}
	m := &Manager{root: root}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

// Reload builds a fresh set and publishes it only when every file is valid.
func (m *Manager) Reload() error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	s, err := m.load()
	if err != nil {
		failure.Log(context.Background(), "templates.background", failure.Wrap("templates.background", err))
		return err
	}
	// 仅在全部文件解析成功后原子替换，失败时旧快照继续生效。
	m.current.Store(s)
	return nil
}

func (m *Manager) load() (*snapshot, error) {
	files, err := filebundle.Select(m.root, builtin.FS())
	if err != nil {
		return nil, err
	}
	if err = filebundle.Validate(files, builtin.FS(), m.root); err != nil {
		return nil, err
	}
	// 按目录约定分类：layouts/partials 为公共模板，pages 为页面，text 为文本模板。
	var common, pages, texts []string
	err = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		switch {
		case strings.HasPrefix(name, "html/layouts/") || strings.HasPrefix(name, "html/partials/"):
			if strings.HasSuffix(name, ".html") {
				common = append(common, name)
			}
		case strings.HasPrefix(name, "html/pages/") && strings.HasSuffix(name, ".html"):
			pages = append(pages, name)
		case strings.HasPrefix(name, "text/") && (strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".tmpl")):
			texts = append(texts, name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan templates %s: %w", m.root, err)
	}

	// missingkey=error 让缺失变量直接报错，避免渲染出半成品页面。
	s := &snapshot{pages: make(map[string]*html.Template), text: text.New("text").Option("missingkey=error")}
	for _, page := range pages {
		t := html.New("__page_root").Option("missingkey=error")
		// 每个页面都重建公共模板，确保页面级 define 不会相互污染。
		for _, name := range append(append([]string{}, common...), page) {
			content, err := fs.ReadFile(files, name)
			if err != nil {
				return nil, err
			}
			if _, err = t.New(name).Parse(string(content)); err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
		}
		// 遍历语法树，提前拒绝引用了未定义模板的页面。
		for _, part := range t.Templates() {
			if err := references(part.Tree, func(name string) bool { return t.Lookup(name) != nil }); err != nil {
				return nil, fmt.Errorf("HTML page %s: %w", page, err)
			}
		}
		key := strings.TrimSuffix(strings.TrimPrefix(page, "html/pages/"), ".html")
		s.pages[key] = t
	}

	// 文本模板共用一个命名空间，文件名去掉 text/ 前缀作为模板名。
	for _, name := range texts {
		content, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		if _, err = s.text.New(strings.TrimPrefix(name, "text/")).Parse(string(content)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
	}

	// 文本模板同样校验引用完整性。
	for _, part := range s.text.Templates() {
		if err := references(part.Tree, func(name string) bool { return s.text.Lookup(name) != nil }); err != nil {
			return nil, fmt.Errorf("text template %s: %w", part.Name(), err)
		}
	}
	return s, nil
}

// RenderHTML accepts a path relative to html/pages without the .html suffix.
func (m *Manager) RenderHTML(w io.Writer, page string, data any) error {
	t := m.current.Load().pages[page]
	if t == nil {
		return fmt.Errorf("unknown HTML page %q", page)
	}
	// 先渲染到缓冲，失败时不向调用方写入任何字节。
	var out bytes.Buffer
	if err := t.ExecuteTemplate(&out, "html/pages/"+page+".html", data); err != nil {
		return err
	}
	_, err := out.WriteTo(w)
	return err
}

// RenderText accepts a path relative to text, including its file extension,
// or an explicitly defined template name.
func (m *Manager) RenderText(w io.Writer, name string, data any) error {
	var out bytes.Buffer
	if err := m.current.Load().text.ExecuteTemplate(&out, name, data); err != nil {
		return err
	}
	_, err := out.WriteTo(w)
	return err
}

func Export(dataDir string, overwrite bool) (filebundle.ExportResult, error) {
	return filebundle.Export(builtin.FS(), filepath.Join(dataDir, "templates"), overwrite)
}

// references 递归遍历模板语法树，检查所有 template 引用均已定义。
func references(tree *parse.Tree, exists func(string) bool) error {
	if tree == nil {
		return nil
	}
	var visit func(parse.Node) error
	visit = func(node parse.Node) error {
		switch n := node.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					if err := visit(child); err != nil {
						return err
					}
				}
			}
		case *parse.TemplateNode:
			if !exists(n.Name) {
				return fmt.Errorf("undefined template %q", n.Name)
			}
		// 分支节点需同时检查主分支与 else 分支。
		case *parse.IfNode:
			if err := visit(n.List); err != nil {
				return err
			}
			return visit(n.ElseList)
		case *parse.RangeNode:
			if err := visit(n.List); err != nil {
				return err
			}
			return visit(n.ElseList)
		case *parse.WithNode:
			if err := visit(n.List); err != nil {
				return err
			}
			return visit(n.ElseList)
		}
		return nil
	}
	return visit(tree.Root)
}
