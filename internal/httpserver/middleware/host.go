package middleware

import (
	"net/http"
	"strings"

	"isekai-mcp-gateway/internal/config"
)

func Host(c *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 允许列表匹配不区分大小写，未命中直接拒绝
		for _, host := range c.Static.Server.AllowedHosts {
			if strings.EqualFold(host, r.Host) {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "Host is not allowed", http.StatusForbidden)
	})
}
