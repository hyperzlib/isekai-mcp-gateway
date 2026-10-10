package middleware

import (
	"bytes"
	"context"
	"io"
	"isekai-mcp-gateway/internal/config"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"isekai-mcp-gateway/internal/failure"
)

func Logging(next http.Handler, routes ...chi.Routes) http.Handler {
	return logging(next, nil, routes...)
}

func logging(next http.Handler, traceEnabled func() bool, routes ...chi.Routes) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 生成服务端请求 ID 并回填请求/响应头；Header 克隆避免改动原请求
		ctx := failure.NewRequest(r.Context())
		r = r.WithContext(ctx)
		r.Header = r.Header.Clone()
		r.Header.Set("X-Request-ID", failure.RequestID(ctx))
		w.Header().Set("X-Request-ID", failure.RequestID(ctx))
		writer := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// 注入 chi 路由上下文，使日志能记录匹配的路由模板
		route := chi.NewRouteContext()
		if len(routes) != 0 {
			route.Routes = routes[0]
		} else if router, ok := next.(chi.Routes); ok {
			route.Routes = router
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, route))
		start := time.Now()
		var requestBody, responseBody bytes.Buffer
		trace := traceEnabled != nil && traceEnabled() && slog.Default().Enabled(ctx, slog.LevelDebug)
		if trace {
			slog.DebugContext(ctx, "http exchange request", "request_id", failure.RequestID(ctx), "method", r.Method, "url", r.URL.String(), "host", r.Host, "protocol", r.Proto, "headers", r.Header.Clone(), "content_length", r.ContentLength)
			if r.Body != nil {
				r.Body = &traceBody{Reader: io.TeeReader(r.Body, &requestBody), Closer: r.Body}
			}
			writer.Tee(&responseBody)
		}
		// Queries, headers, bodies and provider errors can contain credentials.
		defer func() {
			// 未显式写入状态码时按 200 记录
			status := writer.Status()
			if status == 0 {
				status = http.StatusOK
			}
			if trace {
				slog.DebugContext(ctx, "http exchange response", "request_id", failure.RequestID(ctx), "method", r.Method, "url", r.URL.String(), "status", status, "duration_ms", time.Since(start).Milliseconds(), "request_body", requestBody.String(), "request_trailers", r.Trailer, "response_headers", writer.Header().Clone(), "response_body", responseBody.String())
			}
			slog.DebugContext(ctx, "http request", "request_id", failure.RequestID(ctx), "method", r.Method,
				"route", route.RoutePattern(), "status", status, "duration_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(writer, r)
	})
}

type traceBody struct {
	io.Reader
	io.Closer
}

func requestTraceEnabled(c *config.Config) bool {
	if current := config.GetConfig(); current != nil {
		return current.App.Logging.TraceRequest
	}
	return c.App.Logging.TraceRequest
}
