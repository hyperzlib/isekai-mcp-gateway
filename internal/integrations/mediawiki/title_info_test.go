package mediawiki

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

func TestTitleInfoURLMapping(t *testing.T) {
	c := New(config.Defaults())
	c.Config.PublicURL = "https://target.example"
	c.Config.PublicOrigins = []string{"https://ALIAS.example:443/"}
	for _, input := range []string{"/首页", "https://target.example/首页", "https://alias.example/首页"} {
		u, err := c.titleInfoURL(input)
		if err != nil || u.String() != "https://target.example/%E9%A6%96%E9%A1%B5?action=isekai-title-info" {
			t.Fatalf("%s: %v %v", input, u, err)
		}
	}
	u, err := c.titleInfoURL("https://alias.example/%E9%A6%96%E9%A1%B5%2Fchild?title=%E9%A6%96%E9%A1%B5&curid=1&oldid=2&action=delete&action=view&veaction=edit&x=a%2Bb#section")
	if err != nil || u.EscapedPath() != "/%E9%A6%96%E9%A1%B5%2Fchild" || u.Fragment != "" || u.Query().Get("veaction") != "" || len(u.Query()["action"]) != 1 || u.Query().Get("action") != "isekai-title-info" || u.Query().Get("title") != "首页" || u.Query().Get("curid") != "1" || u.Query().Get("oldid") != "2" || u.Query().Get("x") != "a+b" {
		t.Fatalf("query/path mapping: %v %v", u, err)
	}
	for _, input := range []string{"", "首页", "//evil.example/首页", "https://evil.example/首页", "https://target.example.evil/首页", "http://target.example/首页", "https://user@target.example/首页", "/\\evil", "/%5Cevil", "https://target.example:444/首页", "/bad%zz", "/?a=%zz", "/?a=1;b=2", "/\nfoo", "javascript:alert(1)"} {
		_, err := c.titleInfoURL(input)
		if err == nil || failure.Describe(err).Code != "INVALID_REQUEST" {
			t.Fatalf("accepted %s: %v", input, err)
		}
	}
	c.Config.PublicURL = "https://updated.example"
	c.Config.PublicOrigins = []string{"https://newalias.example"}
	u, err = c.titleInfoURL("https://newalias.example/index.php?curid=1")
	if err != nil || u.Host != "updated.example" {
		t.Fatal("updated configuration ignored", err)
	}
	if _, err = c.titleInfoURL("https://alias.example/首页"); err == nil {
		t.Fatal("stale origin allowed")
	}
	c.Config.PublicURL = ""
	_, err = c.TitleInfo(context.Background(), "/首页")
	if failure.Describe(err).Code != "FEATURE_UNAVAILABLE" {
		t.Fatal(err)
	}
}

func TestTitleInfoResponseAndNoCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"exists", `{"title":"首页","page_id":1,"exists":true,"extra":"ignored"}`, "", 200},
		{"missing", `{"title":"未创建","page_id":0,"exists":false}`, "", 200},
		{"html", `<html>error</html>`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"missing field", `{"title":"首页","page_id":1}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"null", `{"title":"首页","page_id":null,"exists":true}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"wrong type", `{"title":"首页","page_id":"1","exists":true}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"fraction", `{"title":"首页","page_id":1.5,"exists":true}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"negative", `{"title":"首页","page_id":-1,"exists":false}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"inconsistent", `{"title":"首页","page_id":1,"exists":false}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"blank", `{"title":" ","page_id":0,"exists":false}`, "UPSTREAM_RESPONSE_INVALID", 200},
		{"oversize", strings.Repeat("x", 65537), "UPSTREAM_RESPONSE_TOO_LARGE", 200},
		{"redirect", "", "UPSTREAM_ERROR", 302},
		{"server error", "", "UPSTREAM_ERROR", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.App.MediaWiki.PublicURL = "https://wiki.test"
			c := New(cfg)
			jar, _ := cookiejar.New(nil)
			base, _ := url.Parse(cfg.App.MediaWiki.PublicURL)
			jar.SetCookies(base, []*http.Cookie{{Name: "private", Value: "cookie"}})
			c.HTTP.Jar = jar
			calls := 0
			c.HTTP.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				for _, header := range []string{"Signature", "Signature-Input", "X-Isekai-User-Id", "X-Isekai-User-Token", "Cookie", "Authorization"} {
					if r.Header.Get(header) != "" {
						t.Fatalf("credential sent: %s", header)
					}
				}
				if r.Method != "GET" || r.URL.Host != "wiki.test" {
					t.Fatalf("unexpected request: %v", r)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Location": []string{"https://evil.test/"}}, Body: io.NopCloser(strings.NewReader(tc.body)), Request: r}, nil
			})
			out, err := c.TitleInfo(context.Background(), "/index.php?curid=1")
			if tc.code != "" {
				if err == nil || failure.Describe(err).Code != tc.code {
					t.Fatalf("want %s got %v", tc.code, err)
				}
			} else if err != nil || out.Title == "" || out.Exists != (out.PageID > 0) {
				t.Fatalf("%#v %v", out, err)
			}
			if calls != 1 || c.HTTP.Jar != jar {
				t.Fatal("redirect followed or shared client mutated")
			}
		})
	}
	c := New(config.Defaults())
	c.Config.PublicURL = "https://wiki.test"
	c.HTTP.Transport = testTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	_, err := c.TitleInfo(context.Background(), "/首页")
	if failure.Describe(err).Code != "API_NETWORK_ERROR" {
		t.Fatal(err)
	}
}
