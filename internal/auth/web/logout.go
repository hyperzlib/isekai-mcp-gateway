package web

import "net/http"

// Logout 仅清除 gateway_session cookie：不吊销 session 记录与 grants，
// 现有令牌校验依赖 session 有效性，因此 agent 客户端的 access/refresh
// token 不受影响。IDP 端 token 未落库，本次不做 IDP 端登出。
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.cookie(w, "gateway_session", "", -1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
