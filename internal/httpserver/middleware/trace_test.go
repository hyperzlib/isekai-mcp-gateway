package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTracePreservesStreaming(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	for _, enabled := range []bool{false, true} {
		output.Reset()
		response := httptest.NewRecorder()
		handler := logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "private-request" {
				t.Fatalf("request body: %q, %v", body, err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Set-Cookie", "private-cookie")
			io.WriteString(w, "data: first\n\n")
			w.(http.Flusher).Flush()
			if !response.Flushed || response.Body.String() != "data: first\n\n" {
				t.Fatal("stream was buffered")
			}
			if strings.Contains(output.String(), "http exchange response") {
				t.Fatal("response logged before closure")
			}
			io.WriteString(w, "data: second\n\n")
		}), func() bool { return enabled })
		request := httptest.NewRequest("POST", "/mcp?private-query", strings.NewReader("private-request"))
		request.Header.Set("Authorization", "Bearer private-token")
		handler.ServeHTTP(response, request)
		for _, value := range []string{"private-query", "private-request", "private-cookie", "private-token", "data: first", "data: second", "http exchange response"} {
			if strings.Contains(output.String(), value) != enabled {
				t.Fatalf("trace enabled=%v, missing or unexpected %q", enabled, value)
			}
		}
	}
}
