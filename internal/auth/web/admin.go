package web

import (
	"errors"
	"net/http"
	"slices"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

// RequireAdmin protects future system administration routes. Personal management
// routes use browser sessions but do not require administrator privileges.
func (s *Service) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := s.browserSession(r, s.browser(w, r))
		if err != nil {
			if errors.Is(err, application.ErrNoSession) {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			} else {
				s.flowError(w, r, err)
			}
			return
		}

		var account application.Account
		row, err := s.Store.Get(s.Store.Database().WithContext(r.Context()), "accounts", session.AccountID, &account, false)
		if err != nil {
			if errors.Is(err, application.ErrRecordNotFound) {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			} else {
				s.flowError(w, r, failure.Wrap("admin.account", err))
			}
			return
		}
		if row.State != "active" || account.ID != session.AccountID {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		if s.Upstream == nil {
			s.flowError(w, r, failure.New("ADMIN_CONFIG_UNAVAILABLE", "管理员配置不可用。", "admin.config", nil))
			return
		}
		bundle := s.Upstream.Get()
		if bundle == nil || bundle.Config == nil {
			s.flowError(w, r, failure.New("ADMIN_CONFIG_UNAVAILABLE", "管理员配置不可用。", "admin.config", nil))
			return
		}
		issuer := bundle.Config.App.Auth.OidcClient.Issuer
		if issuer == "" || account.Issuer != issuer || account.Subject == "" || !slices.Contains(s.Config.Static.Admin.Subs, account.Subject) {
			s.pageErrorContext(r.Context(), w, "没有访问管理页面的权限。", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
