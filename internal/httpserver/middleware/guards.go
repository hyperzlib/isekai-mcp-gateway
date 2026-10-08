package middleware

import (
	"github.com/go-chi/chi/v5"
	"net/http"

	"isekai-mcp-gateway/internal/config"
)

func Guards(c *config.Config, next http.Handler) http.Handler {
	// 由内到外依次应用请求体、Origin、Host、安全头中间件
	guarded := Headers(Host(c, Origin(c, BodyLimit(c, next))))

	// 下游为 chi 路由时注入，便于日志解析匹配的路由模板
	if routes, ok := next.(chi.Routes); ok {
		return Logging(guarded, routes)
	}
	return Logging(guarded)
}
