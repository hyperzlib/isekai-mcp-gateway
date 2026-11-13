package web

import (
	"errors"
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
)

func (s *Service) beginLogin(w http.ResponseWriter, r *http.Request, interaction, browser string) {
	location, err := s.BeginLogin(r.Context(), interaction, browser)
	if err != nil {
		// 发起登录失败时提供“重试”入口，指向发起本次登录的端点。
		var flow *application.FlowError
		if errors.As(err, &flow) && flow.Retry == "" {
			flow.Retry = "/auth/login"
			if interaction != "" {
				flow.Retry = "/auth/interaction/" + interaction
			}
		}
		s.flowError(w, r, err)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (s *Service) ManagementLogin(w http.ResponseWriter, r *http.Request) {
	browser := s.browser(w, r)
	if r.Method == "POST" {
		s.beginLogin(w, r, "", browser)
		return
	}
	s.render(w, r, pageData{Title: "登录", Mode: "login"})
}
