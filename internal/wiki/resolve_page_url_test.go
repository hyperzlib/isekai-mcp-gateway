package wiki

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResolvePageURLAuthorizationAndOutput(t *testing.T) {
	s, c, _ := additionalReadFixture(t, nil)
	c.Client.Config.PublicURL = "https://public.test"
	calls := 0
	c.Client.HTTP.Transport = pageReadTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Signature") != "" || r.Header.Get("X-Isekai-User-Token") != "" {
			t.Fatal("Bridge credentials leaked")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"title":"首页","page_id":1,"exists":true}`))}, nil
	})
	out, err := s.Read(context.Background(), c, "resolve_page_url", Input{URL: "/首页"})
	if err != nil || len(out) != 3 || out["title"] != "首页" || out["page_id"] != int64(1) || out["exists"] != true {
		t.Fatalf("%#v %v", out, err)
	}
	c.Principal.Scopes = nil
	_, err = s.Read(context.Background(), c, "resolve_page_url", Input{URL: "/首页"})
	assertReadCode(t, err, "INSUFFICIENT_SCOPE")
	if calls != 1 {
		t.Fatal("unauthorized request")
	}
}
