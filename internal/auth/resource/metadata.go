package resource

import (
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
)

func (s *Service) ResourceMetadata(w http.ResponseWriter, r *http.Request) {
	g := s.Config.Static.Auth.Gateway
	writeJSON(w, r, 200, map[string]any{"resource": g.Resource, "authorization_servers": []string{g.Issuer}, "scopes_supported": application.BusinessScopes(), "bearer_methods_supported": []string{"header"}})
}
