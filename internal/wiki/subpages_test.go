package wiki

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestSubpagesPagination(t *testing.T) {
	parentTitle := "模板:人物:设定"
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("prop") == "info" {
			if q.Get("redirects") != "" || q.Get("rvprop") != "" {
				t.Fatal("parent query reads content or follows redirects")
			}
			return Object{"query": Object{"pages": []any{Object{"pageid": 3, "ns": 10, "title": parentTitle}}}}
		}
		if q.Get("apnamespace") != "10" || q.Get("apprefix") != "人物:设定/" || q.Get("aplimit") != "2" || q.Get("apdir") != "ascending" || q.Get("apfilterredir") != "all" {
			t.Fatalf("wrong indexed query: %v", q)
		}
		if q.Get("apcontinue") != "" {
			if q.Get("continue") != "-||" {
				t.Fatal("lost generic continuation")
			}
			return Object{"query": Object{"allpages": []any{Object{"pageid": 6, "ns": 10, "title": parentTitle + "/乙"}}}}
		}
		return Object{"query": Object{"allpages": []any{Object{"pageid": 4, "ns": 10, "title": parentTitle + "/甲"}, Object{"pageid": 5, "ns": 10, "title": parentTitle + "/甲/经历", "redirect": true}}}, "continue": Object{"continue": "-||", "apcontinue": "人物:设定/乙"}}
	})
	i := Input{Title: "Template:人物:设定", Limit: 2}
	out, err := s.Read(context.Background(), c, "list_subpages", i)
	if err != nil {
		t.Fatal(err)
	}
	items := out["results"].([]any)
	if out["title"] != parentTitle || out["requested_title"] != i.Title || len(items) != 2 || object(items[0])["relative_title"] != "甲" || object(items[1])["depth"] != 2 || object(items[1])["redirect"] != true || object(items[0])["resources"] == nil {
		t.Fatalf("unexpected output: %#v", out)
	}
	i.Cursor = str(out["next_cursor"])
	out, err = s.Read(context.Background(), c, "list_subpages", i)
	if err != nil || out["next_cursor"] != nil || len(*requests) != 4 {
		t.Fatalf("continuation: %#v %v", out, err)
	}
	for _, changed := range []Input{{Title: i.Title, Limit: 1, Cursor: i.Cursor}, {Title: "Other", Limit: 2, Cursor: i.Cursor}, {PageID: 3, Limit: 2, Cursor: i.Cursor}, {Title: i.Title, Limit: 2, Cursor: i.Cursor + "x"}} {
		_, err = s.Read(context.Background(), c, "list_subpages", changed)
		assertReadCode(t, err, "INVALID_CURSOR")
	}
	parentTitle = "模板:移动后"
	_, err = s.Read(context.Background(), c, "list_subpages", i)
	assertReadCode(t, err, "INVALID_CURSOR")
}

func TestSubpagesMissingParentAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent Object
		input  Input
		list   Object
		code   string
	}{
		{"missing title", Object{"ns": 0, "title": "世界:设定", "missing": true}, Input{Title: "世界:设定"}, Object{"query": Object{"allpages": []any{Object{"pageid": 4, "ns": 0, "title": "世界:设定/人物/甲"}}}}, ""},
		{"empty", Object{"ns": 0, "title": "世界", "pageid": 3}, Input{PageID: 3}, Object{"query": Object{"allpages": []any{}}}, ""},
		{"missing id", Object{"pageid": 3, "missing": true}, Input{PageID: 3}, nil, "NOT_FOUND"},
		{"invalid title", Object{"invalid": true}, Input{Title: "坏标题"}, nil, "INVALID_REQUEST"},
		{"metadata missing", Object{"pageid": 3, "title": "世界"}, Input{PageID: 3}, nil, "UPSTREAM_RESPONSE_INVALID"},
		{"wrong id", Object{"pageid": 4, "ns": 0, "title": "世界"}, Input{PageID: 3}, nil, "UPSTREAM_RESPONSE_INVALID"},
		{"similar prefix", Object{"pageid": 3, "ns": 0, "title": "世界"}, Input{PageID: 3}, Object{"query": Object{"allpages": []any{Object{"pageid": 4, "ns": 0, "title": "世界介绍/甲"}}}}, "UPSTREAM_RESPONSE_INVALID"},
		{"upstream error", Object{"pageid": 3, "ns": 0, "title": "世界"}, Input{PageID: 3}, Object{"error": Object{"code": "internal_api_error", "info": "failed"}}, "UPSTREAM_ERROR"},
		{"list missing", Object{"pageid": 3, "ns": 0, "title": "世界"}, Input{PageID: 3}, Object{"query": Object{}}, "UPSTREAM_RESPONSE_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
				if q.Get("prop") == "info" {
					return Object{"query": Object{"pages": []any{tc.parent}}}
				}
				if q.Get("aplimit") != "20" {
					t.Fatal("wrong default limit")
				}
				if number(tc.parent["ns"]) == 0 && q.Get("apprefix") != str(tc.parent["title"])+"/" {
					t.Fatal("main namespace colon was stripped")
				}
				return tc.list
			})
			out, err := s.Read(context.Background(), c, "list_subpages", tc.input)
			if tc.code != "" {
				assertReadCode(t, err, tc.code)
				return
			}
			if err != nil || out["next_cursor"] != nil {
				t.Fatalf("%#v %v", out, err)
			}
			if strings.HasPrefix(tc.name, "missing") && out["page_id"] != nil {
				t.Fatal("missing parent has id")
			}
			c.Principal.Scopes = nil
			count := len(*requests)
			_, err = s.Read(context.Background(), c, "list_subpages", tc.input)
			assertReadCode(t, err, "INSUFFICIENT_SCOPE")
			if len(*requests) != count {
				t.Fatal("unauthorized upstream query")
			}
		})
	}
}

func TestSubpagesParentMoveInvalidatesIDCursor(t *testing.T) {
	title := "世界"
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("prop") == "info" {
			return Object{"query": Object{"pages": []any{Object{"pageid": 3, "ns": 0, "title": title}}}}
		}
		return Object{"query": Object{"allpages": []any{Object{"pageid": 4, "ns": 0, "title": title + "/甲"}}}, "continue": Object{"continue": "-||", "apcontinue": title + "/乙"}}
	})
	i := Input{PageID: 3, Limit: 1}
	out, err := s.Read(context.Background(), c, "list_subpages", i)
	if err != nil {
		t.Fatal(err)
	}
	i.Cursor = str(out["next_cursor"])
	title = "移动后的世界"
	_, err = s.Read(context.Background(), c, "list_subpages", i)
	assertReadCode(t, err, "INVALID_CURSOR")
	if len(*requests) != 3 {
		t.Fatal("queried a different subtree after parent move")
	}
}
