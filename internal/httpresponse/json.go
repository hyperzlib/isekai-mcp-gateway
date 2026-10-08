package httpresponse

import (
	"context"
	"encoding/json"
	"net/http"

	"isekai-mcp-gateway/internal/failure"
)

// JSON encodes bounded responses before committing headers. A write failure
// after commitment must not attempt a second response.
func JSON(ctx context.Context, w http.ResponseWriter, status int, value any) {
	// 先完成编码，只有成功后才写出状态码，避免半成品响应。
	body, err := json.Marshal(value)
	if err != nil {
		failure.Log(ctx, "http.encode", failure.Wrap("http.encode", err))
		status, body = http.StatusInternalServerError, []byte(`{"error":"server_error"}`)
	}
	// 响应不得缓存；删除可能残留的 Content-Length，交由运行时计算。
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("Content-Length")
	w.WriteHeader(status)
	// 头部已提交，写出失败只能记录，不能再改状态码。
	if _, err := w.Write(append(body, '\n')); err != nil {
		failure.Log(ctx, "http.write", failure.Wrap("http.write", err))
	}
}
