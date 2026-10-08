package health

import (
	"context"
	"net/http"
	"time"
)

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	// 限制依赖健康检查耗时，避免探测请求长时间阻塞
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// 出站未就绪或依赖不健康时返回 503
	if !h.OutboundReady() || h.Dependencies.Health(ctx) != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
