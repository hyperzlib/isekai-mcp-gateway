package middleware

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"isekai-mcp-gateway/internal/config"
)

func Origin(c *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅校验非导航类请求的 Origin；浏览器导航交由表单 CSRF 保护
		if origin := r.Header.Get("Origin"); origin != "" && !browserNavigation(r) {
			// 解析自身 PublicURL 得到用于比对的源
			public, err := url.Parse(c.Static.Server.PublicURL)
			own := ""
			if err == nil {
				own = public.Scheme + "://" + public.Host
			}

			// Origin 必须等于自身或命中白名单
			if origin != own && !slices.Contains(c.Static.Server.AllowedOrigins, origin) {
				http.Error(w, "Origin is not allowed", http.StatusForbidden)
				return
			}

			// 为 MCP 端点设置 CORS 头，支持浏览器客户端
			if r.URL.Path == "/mcp" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version, Mcp-Session-Id, Last-Event-ID")
				w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id, WWW-Authenticate, MCP-Protocol-Version")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")

				// 预检请求无需继续处理
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// OAuth browser GET navigations can arrive through cross-origin redirects.
// POSTs still require both the Origin allowlist and the form CSRF checks.
func browserNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}

	// 这些 OAuth 相关路径允许经跨站重定向到达的 GET 导航
	switch r.URL.Path {
	case "/auth/login", "/auth/oidc_client/callback", "/auth/clients", "/auth/grants", "/oauth/authorize", "/oauth/authorize/callback":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/auth/interaction/")
}
