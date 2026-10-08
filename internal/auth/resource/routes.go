package resource

import (
	"github.com/go-chi/chi/v5"
)

func (s *Service) Routes(r chi.Router) {
	r.Get("/.well-known/oauth-protected-resource", s.ResourceMetadata)
	r.Get("/.well-known/oauth-protected-resource/mcp", s.ResourceMetadata)
}
