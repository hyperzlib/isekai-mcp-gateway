package zitadel

import (
	"errors"
	"net/http"
	"net/url"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

func (s *Service) Introspect(w http.ResponseWriter, r *http.Request) {
	// 优先从 Basic 认证取 client_id，否则回退到表单参数
	id, _, ok := r.BasicAuth()
	if ok {
		var err error
		id, err = url.QueryUnescape(id)
		if err != nil {
			writeJSON(w, r, 400, map[string]string{"error": "invalid_request"})
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			writeJSON(w, r, 400, map[string]string{"error": "invalid_request"})
			return
		}
		id = r.Form.Get("client_id")
	}
	c, _, err := s.LoadClient(s.Store.Database().WithContext(r.Context()), id, false)
	// 基础设施错误上报 server_error，其余情况按访问拒绝处理
	if err != nil && !errors.Is(err, application.ErrRecordNotFound) && failure.IsInfrastructure(err) {
		failure.Log(r.Context(), "oauth.introspect", err)
		writeJSON(w, r, 500, map[string]string{"error": "server_error"})
		return
	}

	// 仅允许已注册的资源服务器客户端执行自省
	if err != nil || !c.ResourceClient || c.TokenAuthMethod == "none" {
		writeJSON(w, r, 403, map[string]string{"error": "access_denied"})
		return
	}
	s.providerResponse(w, r, false)
}
