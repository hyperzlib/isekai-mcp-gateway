package web

import (
	"net/http"
)

func (s *Service) beginLogin(w http.ResponseWriter, r *http.Request, interaction, browser string) {
	location, err := s.BeginLogin(r.Context(), interaction, browser)
	if err != nil {
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
