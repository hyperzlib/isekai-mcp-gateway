package httpresponse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type brokenJSON struct{}

func (brokenJSON) MarshalJSON() ([]byte, error) { return nil, errors.New("secret internal payload") }

func TestEncodingBeforeCommit(t *testing.T) {
	w := httptest.NewRecorder()
	// 预置错误的 Content-Length，验证编码失败会回退 500 并清除它。
	w.Header().Set("Content-Length", "999")
	JSON(context.Background(), w, 201, brokenJSON{})
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Content-Length") != "" {
		t.Fatal("encoding committed a success or exposed its cause")
	}
}

type failingWriter struct {
	header   http.Header
	statuses []int
	writes   int
}

func (w *failingWriter) Header() http.Header { return w.header }
func (w *failingWriter) WriteHeader(n int)   { w.statuses = append(w.statuses, n) }
func (w *failingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("secret disconnected writer")
}
func TestWriteFailureDoesNotSendSecondResponse(t *testing.T) {
	w := &failingWriter{header: make(http.Header)}
	JSON(context.Background(), w, 200, map[string]string{"ok": "yes"})
	if len(w.statuses) != 1 || w.statuses[0] != 200 || w.writes != 1 {
		t.Fatal("write failure attempted another response")
	}
}
