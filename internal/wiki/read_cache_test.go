package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func expireReadCache(s *Service) {
	s.pageCache.mu.Lock()
	defer s.pageCache.mu.Unlock()
	for key, entry := range s.pageCache.entries {
		entry.expires = time.Now().Add(-time.Second)
		s.pageCache.entries[key] = entry
	}
}

func TestPageContinuationCacheRefreshExpiryAndAccess(t *testing.T) {
	for _, format := range []string{"source", "html"} {
		t.Run(format, func(t *testing.T) {
			latest := 10
			readable, visible := true, true
			contents := map[string]string{"10": "abcdefgh", "11": "new-page"}
			parses, fullReads, checks := 0, 0, 0
			s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
				if q.Get("action") == "parse" {
					parses++
					if q.Get("prop") == "images" {
						return Object{"parse": Object{"images": []any{}}}
					}
					return Object{"parse": Object{"text": "<p>" + contents[q.Get("oldid")] + "</p>"}}
				}
				id := q.Get("revids")
				if id == "" {
					id = fmt.Sprint(latest)
				}
				revision := Object{"revid": 10, "slots": Object{"main": Object{}}}
				if id == "11" {
					revision["revid"] = 11
				}
				slot := object(object(revision["slots"])["main"])
				if strings.Contains(q.Get("rvprop"), "content") {
					fullReads++
					slot["content"], slot["contentmodel"] = contents[id], "wikitext"
				}
				if q.Get("intestactions") == "read" {
					checks++
					if visible {
						slot["sha1"] = "visible"
					}
				}
				actions := Object{"read": []any{}}
				if !readable {
					actions["read"] = []any{Object{"code": "permissiondenied"}}
				}
				return Object{"query": Object{"pages": []any{Object{"pageid": 3, "title": "Page", "actions": actions,
					"revisions": []any{revision}, "isekaiuseredited": Object{"has_edited": 1}}}}}
			})
			input := Input{Title: "Page", Format: format, Limit: 3}
			first, err := s.Read(context.Background(), c, "get_latest_page", input)
			if err != nil {
				t.Fatal(err)
			}
			if len(*requests) != 2 || !strings.Contains((*requests)[0].Get("prop"), "isekaiuseredited") {
				t.Fatalf("unmerged first read: %v", *requests)
			}
			input.Cursor = str(first["next_cursor"])
			before := parses
			second, err := s.Read(context.Background(), c, "get_latest_page", input)
			if err != nil || number(second["offset"]) != 3 || fullReads != 1 || parses != before || checks != 1 {
				t.Fatalf("cached continuation: %#v %v, reads=%d parses=%d checks=%d", second, err, fullReads, parses, checks)
			}
			// Access revocation and revision hiding are checked even on a cache hit.
			readable = false
			_, err = s.Read(context.Background(), c, "get_latest_page", input)
			assertReadCode(t, err, "PERMISSION_DENIED")
			readable, visible = true, false
			_, err = s.Read(context.Background(), c, "get_latest_page", input)
			assertReadCode(t, err, "PERMISSION_DENIED")
			visible = true
			expireReadCache(s)
			_, err = s.Read(context.Background(), c, "get_latest_page", input)
			if err != nil || fullReads != 2 {
				t.Fatalf("expired read: %v, reads=%d", err, fullReads)
			}
			if format == "html" && parses != before+1 {
				t.Fatal("expired HTML was not parsed again")
			}
			// A first-page read sees the new latest revision and replaces this query's cache.
			latest = 11
			fresh, err := s.Read(context.Background(), c, "get_latest_page", Input{Title: "Page", Format: format, Limit: 3})
			if err != nil || number(fresh["revision_id"]) != 11 || fullReads != 3 {
				t.Fatalf("first-page refresh: %#v %v", fresh, err)
			}
			// The old cursor still selects revision 10 after the refresh.
			old, err := s.Read(context.Background(), c, "get_latest_page", input)
			if err != nil || number(old["revision_id"]) != 10 || fullReads != 4 {
				t.Fatalf("old snapshot: %#v %v", old, err)
			}
			input.Cursor = str(fresh["next_cursor"])
			s.invalidatePageCache(c, 3, "Page")
			_, err = s.Read(context.Background(), c, "get_latest_page", input)
			if err != nil || fullReads != 5 {
				t.Fatalf("write invalidation: %v, reads=%d", err, fullReads)
			}
		})
	}
}

func TestReadCacheIsolationTTLAndInFlightInvalidation(t *testing.T) {
	s, c, _ := additionalReadFixture(t, func(url.Values) Object { return nil })
	c.Config.App.MediaWiki.PageCacheTTL = 123
	scope := c.cacheScope("get_latest_page", "filter")
	generation := s.pageCache.begin(scope, true)
	id := s.pageCache.put(c, scope, generation, 3, "Page", Object{"content": "original"})
	if id == "" {
		t.Fatal("cache insertion failed")
	}
	entry := s.pageCache.entries[id]
	if remaining := time.Until(entry.expires); remaining < 122*time.Second || remaining > 123*time.Second {
		t.Fatalf("configured TTL: %v", remaining)
	}
	copy := s.pageCache.get(id, scope)
	copy["content"] = "changed"
	if s.pageCache.get(id, scope)["content"] != "original" {
		t.Fatal("shared mutable cache object")
	}
	other := *c
	other.Credentials.Token = "other-token"
	if s.pageCache.get(id, other.cacheScope("get_latest_page", "filter")) != nil {
		t.Fatal("cache crossed credentials")
	}
	otherPrincipal := *c.Principal
	otherPrincipal.GrantID = "other-grant"
	other = *c
	other.Principal = &otherPrincipal
	if s.pageCache.get(id, other.cacheScope("get_latest_page", "filter")) != nil {
		t.Fatal("cache crossed grants")
	}
	s.invalidatePageCache(c, 3, "Page")
	if s.pageCache.get(id, scope) != nil {
		t.Fatal("page cache survived invalidation")
	}
	if s.pageCache.put(c, scope, generation, 3, "Page", Object{"content": "stale"}) != "" {
		t.Fatal("in-flight read restored invalidated cache")
	}
}
