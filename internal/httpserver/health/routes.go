package health

import (
	"context"

	"github.com/go-chi/chi/v5"
)

type Dependencies interface {
	Health(context.Context) error
}

type Handler struct {
	Dependencies  Dependencies
	OutboundReady func() bool
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/health/live", h.Live)
	r.Get("/health/ready", h.Ready)
}
