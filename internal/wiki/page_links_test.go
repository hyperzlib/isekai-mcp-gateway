package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func pageLinksFixture(t *testing.T, links *[]any) (*Service, *Call, *int) {
	t.Helper()
	parses := new(int)
	s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("action") == "parse" {
			*parses++
			if q.Get("oldid") != "42" {
				t.Fatalf("wrong revision for links: %v", q)
			}
			return Object{"parse": Object{"images": []any{}, "links": *links}}
		}
		pg := Object{"pageid": 3, "title": "页面", "fullurl": "https://wiki.test/页面", "actions": Object{"read": []any{}}, "isekaiuseredited": Object{"has_edited": 0},
			"revisions": []any{Object{"revid": 42, "slots": Object{"main": Object{"content": "abcdefgh", "contentmodel": "wikitext", "sha1": "visible"}}}}}
		return Object{"query": Object{"pages": []any{pg}}}
	})
	return s, c, parses
}

func TestPageLinksCompleteDeduplicatedAndCache(t *testing.T) {
	for _, name := range []string{"get_latest_page", "get_baseline_page"} {
		t.Run(name, func(t *testing.T) {
			links := []any{Object{"title": "模板:展开链接", "exists": true}, Object{"title": "不存在的页面", "exists": false}, Object{"title": "模板:展开链接"}}
			for n := 0; n < 100; n++ {
				links = append(links, Object{"title": fmt.Sprintf("帮助:%03d%s", n, strings.Repeat("长标题", 100))})
			}
			s, c, parses := pageLinksFixture(t, &links)
			c.Config.App.Results.TextMax = 1024 * 1024
			i := Input{Title: "页面", Limit: 2}
			out, err := s.Read(context.Background(), c, name, i)
			if err != nil {
				t.Fatal(err)
			}
			list := out["linked_pages"].([]string)
			if len(list) != 102 || list[0] != "模板:展开链接" || list[1] != "不存在的页面" {
				t.Fatalf("incomplete/reordered links: %v", list)
			}
			if _, exists := out["linked_pages_has_more"]; exists {
				t.Fatal("unexpected truncation field")
			}
			if _, exists := out["linked_pages_hash"]; exists {
				t.Fatal("internal link hash leaked")
			}
			hash := fingerprint(list)
			i.Cursor = str(out["next_cursor"])
			cur, err := readCursor(i.Cursor, name, c.owner(c.Client.Config.SiteID), fingerprint(Object{"page": PageRef{Title: i.Title}, "format": "source", "limit": i.Limit, "follow_redirects": false}))
			if err != nil || len(i.Cursor) > 8192 || cur.Continue["linked_pages"] != nil || cur.Continue["linked_pages_hash"] != hash {
				t.Fatalf("oversized/malformed cursor: %d %v", len(i.Cursor), err)
			}
			out, err = s.Read(context.Background(), c, name, i)
			if err != nil || fingerprint(out["linked_pages"]) != hash || *parses != 1 || str(out["content"]) != "cd" {
				t.Fatalf("cached links: %#v %v parses=%d", out, err, *parses)
			}
			expireReadCache(s)
			out, err = s.Read(context.Background(), c, name, i)
			if err != nil || fingerprint(out["linked_pages"]) != hash || *parses != 2 {
				t.Fatalf("expired links: %v parses=%d", err, *parses)
			}
			links = append(links, Object{"title": "新增模板链接"})
			expireReadCache(s)
			_, err = s.Read(context.Background(), c, name, i)
			assertReadCode(t, err, "SNAPSHOT_UNAVAILABLE")
		})
	}
}

func TestPageLinksEmptyInvalidAndTooLarge(t *testing.T) {
	for _, links := range [][]any{nil, {Object{}}, {"not an object"}, {Object{"title": 42}}, {Object{"title": " "}}} {
		s, c, _ := pageLinksFixture(t, &links)
		_, err := s.Read(context.Background(), c, "get_latest_page", Input{Title: "页面"})
		assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
	}
	links := []any{}
	s, c, _ := pageLinksFixture(t, &links)
	out, err := s.Read(context.Background(), c, "get_latest_page", Input{Title: "页面"})
	if err != nil || len(out["linked_pages"].([]string)) != 0 {
		t.Fatalf("empty links: %#v %v", out, err)
	}
	files, list, err := s.pageSourceMetadata(context.Background(), c, Object{"content": nil}, true)
	if err != nil || len(files) != 0 || list == nil || len(list) != 0 {
		t.Fatalf("missing content: %#v %#v %v", files, list, err)
	}
	links = []any{Object{"title": strings.Repeat("长", 1000)}}
	c.Config.App.Results.TextMax = 2000
	_, err = s.Read(context.Background(), c, "get_latest_page", Input{Title: "页面", Limit: 1})
	assertReadCode(t, err, "RESULT_TOO_LARGE")
}
