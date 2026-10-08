package web

import (
	"html/template"
	"io"
)

type testPages struct{ *template.Template }

func (t testPages) RenderHTML(w io.Writer, _ string, data any) error {
	return t.Execute(w, data)
}
