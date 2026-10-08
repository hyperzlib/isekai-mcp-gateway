package wiki

import (
	"context"
	"fmt"
	"isekai-mcp-gateway/internal/security"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestPageFileURLsAliasesAndBatches(t *testing.T) {
	images := []any{"小 图.png", "别名.png", "缺失.png", "模板.png", "小 图.png"}
	for n := 0; n < 48; n++ {
		images = append(images, fmt.Sprintf("额外%d.png", n))
	}
	batches := 0
	s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("action") == "parse" {
			if q.Get("oldid") != "42" || q.Get("prop") != "images" {
				t.Fatalf("parse: %v", q)
			}
			return Object{"parse": Object{"images": images}}
		}
		batches++
		titles := strings.Split(q.Get("titles"), "|")
		if len(titles) > 50 || q.Get("iiprop") != "url" || q.Get("redirects") != "1" {
			t.Fatalf("query: %v", q)
		}
		pages := []any{}
		for _, title := range titles {
			if title == "File:缺失.png" {
				pages = append(pages, Object{"title": "文件:缺失.png", "missing": true})
				continue
			}
			resolved := strings.Replace(title, "File:", "文件:", 1)
			if title == "File:小 图.png" {
				resolved = "文件:小 图规范.png"
			}
			if title == "File:别名.png" {
				resolved = "文件:目标.png"
			}
			pages = append(pages, Object{"title": resolved, "imageinfo": []any{Object{"url": "https://files.test/" + resolved}}})
		}
		normalized := []any{}
		for _, title := range titles {
			normalized = append(normalized, Object{"from": title, "to": strings.Replace(title, "File:", "文件:", 1)})
		}
		return Object{"query": Object{"pages": pages, "normalized": normalized, "redirects": []any{Object{"from": "文件:小 图.png", "to": "文件:小 图规范.png"}, Object{"from": "文件:别名.png", "to": "文件:目标.png"}}}}
	})
	out, err := s.pageFileURLs(context.Background(), c, Object{"revision_id": 42, "content": "{{模板}}"})
	if err != nil {
		t.Fatal(err)
	}
	if batches != 2 || len(out) != 51 || out["文件:小 图.png"] != "https://files.test/文件:小 图规范.png" || out["文件:别名.png"] != "https://files.test/文件:目标.png" || out["文件:模板.png"] == nil || out["文件:缺失.png"] != nil {
		t.Fatalf("files: %#v; batches %d", out, batches)
	}
}

func TestPageReadFileURLsSnapshotAndPagination(t *testing.T) {
	for _, tool := range []string{"get_latest_page", "get_baseline_page"} {
		t.Run(tool, func(t *testing.T) {
			pending := "待审正文[[文件:待审.png]]"
			baseline := "公开正文[[文件:公开.png]]"
			parses := 0
			s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
				if q.Get("action") == "parse" {
					parses++
					file := "待审.png"
					if tool == "get_baseline_page" {
						file = "公开.png"
						if q.Get("oldid") != "42" {
							t.Fatalf("baseline parse: %v", q)
						}
					} else if q.Get("oldid") != "" || q.Get("text") != pending || q.Get("title") != "测试" || q.Get("contentmodel") != "wikitext" {
						t.Fatalf("pending parse: %v", q)
					}
					return Object{"parse": Object{"images": []any{file}}}
				}
				if q.Get("prop") == "imageinfo" {
					title := q.Get("titles")
					return Object{"query": Object{"pages": []any{Object{"title": title, "imageinfo": []any{Object{"url": "https://files.test/" + title}}}}}}
				}
				pg := Object{"pageid": 3, "title": "测试", "actions": Object{"read": []any{}}, "isekaiuseredited": Object{"has_edited": 0}}
				switch q.Get("prop") {
				case "info|moderationeditstate|isekaiuseredited":
					pg["moderationeditstate"] = Object{"page_id": 3, "title": "测试", "content_source": "pending", "content": pending, "content_hash": security.Hash(pending), "public_revision_id": 42, "temp_revision_id": 9, "chain_version": 1}
				case "moderationrevision":
					pg["moderationrevision"] = Object{"temp_revision_id": 9, "content": pending}
				case "revisions", "revisions|isekaiuseredited", "info|isekaiuseredited|revisions":
					pg["revisions"] = []any{Object{"revid": 42, "slots": Object{"main": Object{"content": baseline, "contentmodel": "wikitext", "sha1": "visible"}}}}
				case "isekaiuseredited", "info|isekaiuseredited":
					pg["isekaiuseredited"] = Object{"has_edited": 0}
				default:
					t.Fatalf("unexpected query %v", q)
				}
				return Object{"query": Object{"pages": []any{pg}}}
			})
			c.Config.Static.Extensions.Moderation.Enabled = true
			input := Input{Title: "测试", Limit: 6}
			combined := ""
			var files Object
			for n := 0; n < 20; n++ {
				out, err := s.Read(context.Background(), c, tool, input)
				if err != nil {
					t.Fatal(err)
				}
				if n == 0 {
					files = object(out["file_urls"])
				} else if !reflect.DeepEqual(files, object(out["file_urls"])) {
					t.Fatal("mapping changed")
				}
				combined += str(out["content"])
				want := pending
				if tool == "get_baseline_page" {
					want = baseline
				}
				if str(out["content_hash"]) != security.Hash(want) || number(out["total_bytes"]) != int64(len(want)) {
					t.Fatalf("metadata: %#v", out)
				}
				input.Cursor = str(out["next_cursor"])
				if input.Cursor == "" {
					break
				}
			}
			want, file := pending, "文件:待审.png"
			if tool == "get_baseline_page" {
				want, file = baseline, "文件:公开.png"
			}
			if combined != want || parses != 1 || len(files) != 1 || files[file] == nil {
				t.Fatalf("content %q files %#v parses %d", combined, files, parses)
			}
		})
	}
}

func TestPageFileURLsEmptyAndInvalidResponse(t *testing.T) {
	for _, valid := range []bool{true, false} {
		s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
			if valid {
				return Object{"parse": Object{"images": []any{}}}
			}
			return Object{"parse": Object{}}
		})
		out, err := s.pageFileURLs(context.Background(), c, Object{"revision_id": 42, "content": "正文"})
		if valid {
			if err != nil || out == nil || len(out) != 0 {
				t.Fatalf("%#v %v", out, err)
			}
		} else {
			assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
		}
	}
}

func TestPageReadHTMLHasNoFileURLs(t *testing.T) {
	s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("action") == "parse" {
			if q.Get("prop") != "text" {
				t.Fatalf("unexpected file parse: %v", q)
			}
			return Object{"parse": Object{"text": "<p>正文</p>"}}
		}
		pg := Object{"pageid": 3, "title": "测试", "revisions": []any{Object{"revid": 42, "slots": Object{"main": Object{"content": "正文", "contentmodel": "wikitext"}}}}}
		if strings.Contains(q.Get("prop"), "isekaiuseredited") {
			pg["isekaiuseredited"] = Object{"has_edited": 0}
		}
		return Object{"query": Object{"pages": []any{pg}}}
	})
	c.Config.Static.Extensions.Moderation.Enabled = false
	out, err := s.Read(context.Background(), c, "get_latest_page", Input{Title: "测试", Format: "html"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["file_urls"]; ok {
		t.Fatal("HTML response includes file mapping")
	}
	if out["content"] != "<p>正文</p>" {
		t.Fatalf("%#v", out)
	}
}

func TestPageFileURLsUpstreamErrors(t *testing.T) {
	for _, failParse := range []bool{true, false} {
		s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
			if !failParse && q.Get("action") == "parse" {
				return Object{"parse": Object{"images": []any{"测试.png"}}}
			}
			return Object{"error": Object{"code": "permissiondenied", "info": "denied"}}
		})
		if _, err := s.pageFileURLs(context.Background(), c, Object{"revision_id": 42, "content": "正文"}); err == nil {
			t.Fatal("upstream error ignored")
		}
	}
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object { t.Fatal("empty baseline reached upstream"); return nil })
	out, err := s.pageFileURLs(context.Background(), c, Object{"content": nil})
	if err != nil || out == nil || len(out) != 0 || len(*requests) != 0 {
		t.Fatalf("%#v %v", out, err)
	}
}

func TestGetFileURLs(t *testing.T) {
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("prop") != "imageinfo" || q.Get("titles") != "File:小 图.png|File:缺失.pdf" {
			t.Fatalf("query %v", q)
		}
		return Object{"query": Object{"normalized": []any{Object{"from": "File:小 图.png", "to": "文件:小 图.png"}}, "pages": []any{Object{"title": "文件:小 图.png", "imageinfo": []any{Object{"url": "https://files.test/image.png"}}}, Object{"title": "File:缺失.pdf", "missing": true}}}}
	})
	out, err := s.Read(context.Background(), c, "get_file_urls", Input{Titles: []string{"文件:小 图.png", "File:小 图.png", "Image:缺失.pdf"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(object(out["file_urls"])) != 1 || object(out["file_urls"])["文件:小 图.png"] != "https://files.test/image.png" {
		t.Fatalf("%#v", out)
	}
	for _, titles := range [][]string{nil, {}, {"正文"}, {"模板:测试"}, {"文件:"}, {"文件:a|b"}, {"文件:a#b"}, make([]string, 501)} {
		_, err := s.Read(context.Background(), c, "get_file_urls", Input{Titles: titles})
		assertReadCode(t, err, "INVALID_REQUEST")
	}
	if len(*requests) != 1 {
		t.Fatal("invalid request reached upstream")
	}
	c.Principal.Scopes = nil
	if _, err := s.Read(context.Background(), c, "get_file_urls", Input{Titles: []string{"文件:小 图.png"}}); err == nil {
		t.Fatal("missing scope accepted")
	}
}
