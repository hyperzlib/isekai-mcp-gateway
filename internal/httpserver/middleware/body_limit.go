package middleware

import (
	"net/http"

	"isekai-mcp-gateway/internal/config"
)

func BodyLimit(c *config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /uploads 的 PUT 由处理器自行限制大小，跳过通用限制
		if r.URL.Path == "/uploads" && r.Method == "PUT" {
			next.ServeHTTP(w, r)
			return
		}

		// 依据 Content-Length 提前拒绝，并用 MaxBytesReader 限制实际读取
		if r.ContentLength > c.Static.Server.RequestBodyMaxBytes {
			http.Error(w, "request body exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, c.Static.Server.RequestBodyMaxBytes)
		next.ServeHTTP(w, r)
	})
}
