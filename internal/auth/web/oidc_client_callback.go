package web

import (
	"net/http"
)

func (s *Service) OidcConnectCallback(w http.ResponseWriter, r *http.Request) {
	// state 必须唯一非空，且 code 与 error 恰好出现其一
	q := r.URL.Query()
	if len(q["state"]) != 1 || q.Get("state") == "" || len(q["code"]) > 1 || len(q["error"]) > 1 ||
		(q.Get("code") == "") == (q.Get("error") == "") {
		s.pageErrorContext(r.Context(), w, "登录回调参数无效。", 400)
		return
	}
	state, browser := q.Get("state"), s.browser(w, r)
	result, err := s.CompleteLogin(r.Context(), state, q.Get("code"), browser, q.Get("error") != "")
	if err != nil {
		s.flowError(w, r, err)
		return
	}
	s.cookie(w, "gateway_session", result.SessionID, s.Config.Static.Auth.Gateway.GrantTTL)
	if result.InteractionID == "" {
		http.Redirect(w, r, "/auth/grants", 303)
		return
	}
	http.Redirect(w, r, "/auth/interaction/"+result.InteractionID, 303)
}
