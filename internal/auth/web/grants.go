package web

import (
	"errors"
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
)

func (s *Service) management(w http.ResponseWriter, r *http.Request, kind, mode, title string) {
	// 管理页要求已登录，无会话则跳转登录
	session, err := s.browserSession(r, s.browser(w, r))
	if err != nil {
		if !errors.Is(err, application.ErrNoSession) {
			s.flowError(w, r, err)
			return
		}
		http.Redirect(w, r, "/auth/login", 303)
		return
	}

	// 仅接受 revoke 动作，撤销后重定向回当前页
	if r.Method == "POST" {
		if r.ParseForm() != nil || r.PostForm.Get("action") != "revoke" {
			s.pageErrorContext(r.Context(), w, "请求无效，请重试。", 400)
			return
		}
		if err = s.RevokeManaged(r.Context(), session.AccountID, kind, r.PostForm.Get("id")); err != nil {
			s.flowError(w, r, err)
			return
		}
		http.Redirect(w, r, r.URL.Path, 303)
		return
	}
	entries, err := s.ListManaged(r.Context(), session.AccountID, kind)
	if err != nil {
		s.flowError(w, r, err)
		return
	}

	// 将存储的 scope 名称转换为可读文本
	for i := range entries {
		entries[i].Scope = scopeSummary(entries[i].Scope)
	}
	s.render(w, r, pageData{Title: title, Mode: mode, Username: session.Identity.DisplayName, Entries: entries})
}
func (s *Service) Grants(w http.ResponseWriter, r *http.Request) {
	s.management(w, r, "grants", "grants", "管理授权")
}
