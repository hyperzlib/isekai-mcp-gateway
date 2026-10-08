package httpserver

import (
	"github.com/go-chi/chi/v5"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/httpserver/middleware"
)

// RouteModule registers its own routes. The HTTP server only composes modules.
type RouteModule interface{ Routes(chi.Router) }

func New(c *config.Config, modules ...RouteModule) *Server {
	r := chi.NewRouter()
	for _, module := range modules {
		module.Routes(r)
	}

	// 用安全中间件包裹路由后交给 HTTP 服务器
	return &Server{Handler: middleware.Guards(c, r)}
}
