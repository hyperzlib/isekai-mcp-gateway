package web

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
)

func (s *Service) Routes(r chi.Router) {
	forms := chi.NewRouter()
	forms.Get("/interaction/{uid}", s.Interaction)
	forms.Post("/interaction/{uid}", s.Interaction)
	forms.Get("/oidc_client/callback", s.OidcConnectCallback)
	forms.Get("/login", s.ManagementLogin)
	forms.Post("/login", s.ManagementLogin)
	forms.Get("/clients", s.Clients)
	forms.Post("/clients", s.Clients)
	forms.Get("/grants", s.Grants)
	forms.Post("/grants", s.Grants)
	// 表单路由统一挂载 CSRF 防护，HTTPS issuer 下才启用 Secure Cookie
	csrf := nosurf.New(forms)
	csrf.SetBaseCookie(http.Cookie{Name: "gateway_csrf", Path: "/auth", HttpOnly: true,
		Secure: strings.HasPrefix(s.Config.Static.Auth.Gateway.Issuer, "https://"), SameSite: http.SameSiteLaxMode})
	csrf.SetFailureHandler(http.HandlerFunc(csrfFailure))
	r.Mount("/auth", csrf)
}

func csrfFailure(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "CSRF validation failed; reopen the authorization page", http.StatusForbidden)
}
