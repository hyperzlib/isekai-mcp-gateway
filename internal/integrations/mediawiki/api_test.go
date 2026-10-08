package mediawiki

import (
	"context"
	"encoding/json"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSignedQueryWriteAndBadToken(t *testing.T) {
	var calls atomic.Int32
	var csrf atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		// 签名须使用用户 token 组件，且不得混入 subject 组件
		if r.Header.Get("X-Isekai-User-Token") != "private-token" || !strings.Contains(r.Header.Get("Signature-Input"), "x-isekai-user-token") || strings.Contains(r.Header.Get("Signature-Input"), "x-isekai-subject") {
			t.Error("user identity signature is invalid")
		}

		// 首次 CSRF 查询返回 token
		if r.Method == "GET" && r.URL.Query().Get("meta") == "tokens" {
			csrf.Add(1)
			_, _ = w.Write([]byte(`{"query":{"tokens":{"csrftoken":"csrf"}}}`))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("action") == "edit" {
			if r.Form.Get("token") != "csrf" || r.Form.Get("maxlag") != "5" {
				t.Error("missing write guards")
			}
			_, _ = w.Write([]byte(`{"error":{"code":"badtoken","info":"private trace"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"search":[]},"continue":{"continue":"-||","sroffset":3}}`))
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.App.MediaWiki.APIURL = server.URL
	cfg.App.MediaWiki.SiteID = t.Name()
	cfg.App.MediaWiki.Bridge.KeyID = "key"
	cfg.Secrets.BridgeSecret = strings.Repeat("s", 32)
	c := New(cfg)
	cred := Credentials{1, "private-token"}
	v, e := c.Query(context.Background(), cred, url.Values{"action": {"query"}, "list": {"search"}})
	if e != nil || v["continue"] == nil {
		t.Fatal(v, e)
	}
	token, e := c.CSRF(context.Background(), cred)
	if e != nil {
		t.Fatal(e)
	}
	// 第二次获取应命中缓存，不再发起请求
	_, e = c.CSRF(context.Background(), cred)
	if e != nil || csrf.Load() != 1 {
		t.Fatal("CSRF not cached")
	}
	_, e = c.Write(context.Background(), cred, url.Values{"action": {"edit"}, "token": {token}})

	// badtoken 映射为 CSRF_TOKEN_INVALID，且不得泄露上游 trace
	if e == nil || failure.Describe(e).Code != "CSRF_TOKEN_INVALID" || strings.Contains(e.Error(), "private trace") {
		t.Fatal(e)
	}

	// badtoken 后缓存被清除，重新获取会再次请求上游
	_, e = c.CSRF(context.Background(), cred)
	if e != nil || csrf.Load() != 2 || calls.Load() != 4 {
		t.Fatalf("badtoken retried or cache not cleared: %d %d", calls.Load(), csrf.Load())
	}
}
func TestNetworkRequestsAreNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.App.MediaWiki.APIURL = server.URL
	cfg.Secrets.BridgeSecret = strings.Repeat("s", 32)
	c := New(cfg)
	_, e := c.Write(context.Background(), Credentials{1, "token"}, url.Values{"action": {"edit"}})
	if e == nil || failure.Describe(e).Code != "API_NETWORK_ERROR" || calls.Load() != 1 {
		t.Fatalf("request count %d error %v", calls.Load(), e)
	}
}
func TestModerationQueuedIsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "moderation-edit-queued", "moderation": map[string]any{"mod_id": 1, "temp_revision_id": 2, "chain_version": 3}}})
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.App.MediaWiki.APIURL = server.URL
	c := New(cfg)
	v, e := c.Write(context.Background(), Credentials{1, "token"}, url.Values{"action": {"edit"}})
	if e != nil || v["error"] == nil {
		t.Fatal(v, e)
	}
}
