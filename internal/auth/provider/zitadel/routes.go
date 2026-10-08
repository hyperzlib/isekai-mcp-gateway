package zitadel

import (
	"github.com/go-chi/chi/v5"
)

func (s *Service) Routes(r chi.Router) {
	r.Get("/.well-known/oauth-authorization-server", s.Discovery)
	r.Get("/.well-known/openid-configuration", s.Discovery)
	r.Get("/oauth/authorize", s.Authorize)
	r.Post("/oauth/authorize", s.Authorize)
	r.Get("/oauth/authorize/callback", s.ProviderCallback)
	r.Post("/oauth/token", s.TokenEndpoint)
	r.Post("/oauth/register", s.Registration)
	r.Post("/oauth/introspect", s.Introspect)
	r.Post("/oauth/revoke", s.Revocation)
	r.Get("/oauth/jwks", s.JWKS)
	r.Get("/oauth/userinfo", s.Userinfo)
	r.Post("/oauth/userinfo", s.Userinfo)
}
