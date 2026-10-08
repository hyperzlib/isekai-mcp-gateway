package web

import (
	"net/http"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/provider/zitadel"
	"isekai-mcp-gateway/internal/templates"
)

type Service struct {
	*application.Service
	Protocol *zitadel.Service
	Pages    templates.HTMLRenderer
}

func New(protocol *zitadel.Service, pages templates.HTMLRenderer) *Service {
	s := &Service{Service: protocol.Service, Protocol: protocol, Pages: pages}
	// 将 Web 层的浏览器标识与错误页实现注入 provider，打通两层的回调
	protocol.Browser = s.browser
	protocol.PageError = func(w http.ResponseWriter, r *http.Request, message string, status int) {
		s.pageErrorContext(r.Context(), w, message, status)
	}
	return s
}
func (s *Service) AuthError(w http.ResponseWriter, r *http.Request, redirect, state, code, description string) {
	s.Protocol.AuthError(w, r, redirect, state, code, description)
}
