package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/templates"
)

func TestRedirectAfterRemoval(t *testing.T) {
	for _, tc := range []struct{ marker, want string }{
		{"0123456789abcdef0123456789abcdef", "/auth/apikeys?removed=0123456789abcdef0123456789abcdef"},
		{"invalid", "/auth/apikeys"},
		{"0123456789abcdef0123456789abcde/", "/auth/apikeys"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/auth/apikeys", strings.NewReader(url.Values{"notice_id": {tc.marker}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		redirectAfterRemoval(w, req)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tc.want {
			t.Fatalf("marker %q: got %d %q", tc.marker, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestRemovalNoticeWithoutJavaScript(t *testing.T) {
	post := httptest.NewRequest(http.MethodPost, "https://gateway.example/auth/apikeys", strings.NewReader(url.Values{"notice_name": {"<测试密钥>"}}.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := post.ParseForm(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	redirectAfterRemoval(w, post)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "gateway_removal_notice" || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("unexpected notice cookie: %#v", cookies)
	}
	pages, err := templates.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Pages: pages}
	get := httptest.NewRequest(http.MethodGet, "https://gateway.example/auth/apikeys", nil)
	get.AddCookie(cookies[0])
	out := httptest.NewRecorder()
	service.render(out, get, pageData{Mode: "apikeys", Title: "API Keys", Wide: true})
	if out.Code != 200 || !strings.Contains(out.Body.String(), "已删除 &lt;测试密钥&gt;") || !strings.Contains(out.Body.String(), "<noscript>") {
		t.Fatalf("missing escaped no-script notice: %d", out.Code)
	}
	cleared := out.Result().Cookies()
	if len(cleared) == 0 || cleared[len(cleared)-1].Name != "gateway_removal_notice" || cleared[len(cleared)-1].MaxAge >= 0 {
		t.Fatalf("notice was not cleared: %#v", cleared)
	}
}
