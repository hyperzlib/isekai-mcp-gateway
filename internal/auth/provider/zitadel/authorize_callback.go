package zitadel

import (
	"net/http"

	"isekai-mcp-gateway/internal/failure"
)

func (s *Service) ProviderCallback(w http.ResponseWriter, r *http.Request) {
	a, row, err := s.LoadInteraction(s.Store.Database().WithContext(r.Context()), r.URL.Query().Get("id"), false)
	if err != nil {
		failure.Log(r.Context(), "oauth.callback", err)
		f := failure.Describe(err)
		s.PageError(w, r, f.Message, failure.HTTPStatus(err))
		return
	}
	// 回调必须来自同一浏览器且授权事务已完成同意，防止跨会话重放
	if row.State != "consented" || !a.DoneFlag || a.Browser != s.Browser(w, r) {
		s.PageError(w, r, "授权事务无效或已完成，请重新连接。", 400)
		return
	}
	s.providerResponse(w, r, false)
}
