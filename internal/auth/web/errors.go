package web

import (
	"errors"
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

// flowError 向浏览器展示受控错误信息，并附带请求编号便于排查
func (s *Service) flowError(w http.ResponseWriter, r *http.Request, err error) {
	failure.Log(r.Context(), "auth.browser", err)
	// 业务流程错误保留其状态码与文案
	var flow *application.FlowError
	if errors.As(err, &flow) {
		s.pageErrorContext(r.Context(), w, flow.Message+errorReference(r), flow.Status)
		return
	}
	f := failure.Describe(err)
	s.pageErrorContext(r.Context(), w, f.Message+errorReference(r), failure.HTTPStatus(f))
}

func errorReference(r *http.Request) string {
	if id := failure.RequestID(r.Context()); id != "" {
		return " 错误编号：" + id
	}
	return ""
}
