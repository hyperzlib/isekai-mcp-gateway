package gatewaymcp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Transport struct{ Handler http.Handler }

func (t *Transport) Routes(r chi.Router) { r.Handle("/mcp", t.Handler) }
