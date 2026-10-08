package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type pageReadTransport func(*http.Request) (*http.Response, error)

func (f pageReadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPageReadUpstreamSelectors(t *testing.T) {
	var queries []url.Values
	transport := pageReadTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		queries = append(queries, q)
		if q.Get("action") == "parse" {
			body, _ := json.Marshal(Object{"parse": Object{"images": []any{}}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
		}
		pg := Object{"pageid": 123, "title": "帮助:编辑/章节", "revisions": []any{Object{"revid": 456, "slots": Object{"main": Object{"content": "正文", "contentmodel": "wikitext"}}}}}
		if strings.Contains(q.Get("prop"), "isekaiuseredited") {
			pg["isekaiuseredited"] = Object{"has_edited": 1}
		}
		body, _ := json.Marshal(Object{"query": Object{"pages": []any{pg}, "backlinks": []any{}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
	cfg := config.Defaults()
	cfg.Static.Extensions.Moderation.Enabled = false
	cfg.App.MediaWiki.APIURL = "https://wiki.test/api.php"
	cfg.Secrets.BridgeSecret = "01234567890123456789012345678901"
	cfg.App.MediaWiki.Bridge.KeyID = "test"
	c := &Call{Principal: &application.Principal{Scopes: []string{"wiki:read"}}, Credentials: mediawiki.Credentials{UserID: 1, Token: "test"}, Client: mediawiki.New(cfg), Config: cfg}
	c.Client.HTTP.Transport = transport
	s := &Service{}
	for _, name := range []string{"get_latest_page", "get_baseline_page", "get_history", "get_backlinks"} {
		for _, input := range []Input{{Title: "帮助:编辑/章节"}, {PageID: 123}} {
			queries = nil
			out, err := s.Read(context.Background(), c, name, input)
			if err != nil {
				t.Fatalf("%s %#v: %v; cause: %v", name, input, err, errors.Unwrap(err))
			}
			key := "titles"
			if name == "get_backlinks" {
				key = "bltitle"
			}
			want := input.Title
			if input.PageID > 0 {
				key = "pageids"
				if name == "get_backlinks" {
					key = "blpageid"
				}
				want = "123"
			}
			if len(queries) == 0 || queries[0].Get(key) != want {
				t.Fatalf("%s: upstream queries %#v", name, queries)
			}
			if name == "get_latest_page" || name == "get_baseline_page" {
				if number(out["page_id"]) != 123 || number(out["revision_id"]) != 456 || object(out["resources"])["by_title"] == nil {
					t.Fatalf("missing resolved metadata: %#v", out)
				}
			}
		}
	}
	queries = nil
	for _, input := range []Input{{}, {Title: "A", PageID: 123}, {Page: PageRef{Title: "A"}}} {
		if _, err := s.Read(context.Background(), c, "get_latest_page", input); err == nil {
			t.Fatalf("accepted %#v", input)
		}
	}
	if len(queries) != 0 {
		t.Fatal("invalid selectors reached upstream")
	}
}

func TestPageResourceSelectors(t *testing.T) {
	template := "wiki://pages/by-title/{encoded_title}/history"
	for _, title := range []string{"帮助:编辑/章节", "123", "A%2FB", "A B+C", "Template:人物信息"} {
		uri := PageResourceLinks(title, 123, "/history")["by_title"].(string)
		i, err := PageResourceInput(uri, template)
		if err != nil || i.Title != title || i.PageID != 0 {
			t.Fatalf("%q: %#v %v", title, i, err)
		}
	}
	i, err := PageResourceInput("wiki://pages/by-id/123/history", "wiki://pages/by-id/{page_id}/history")
	if err != nil || i.PageID != 123 || i.Title != "" {
		t.Fatalf("ID selector: %#v %v", i, err)
	}
	for _, uri := range []string{
		"wiki://pages/by-title/A/B/history", "wiki://pages/by-title/A%ZZ/history",
		"wiki://pages/by-title/A/history?title=B", "wiki://pages/by-title/A/history#x",
		"wiki://pages/by-title/A%7CB/history", "wiki://pages/by-title/%FF/history",
		"wiki://pages/by-title//history", "wiki://pages/123/history",
	} {
		if _, err := PageResourceInput(uri, template); err == nil {
			t.Fatalf("accepted invalid URI %q", uri)
		}
	}
	for _, id := range []string{"0", "-1", "+1", "1%32", "9223372036854775808"} {
		if _, err := PageResourceInput("wiki://pages/by-id/"+id, "wiki://pages/by-id/{page_id}"); err == nil {
			t.Fatalf("accepted ID %q", id)
		}
	}
}
