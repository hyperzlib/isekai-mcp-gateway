package wiki

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
)

func TestPageURLPublicAndMissing(t *testing.T) {
	for _, tc := range []struct {
		name string
		page Object
		want string
		fail bool
	}{
		{"public", Object{"pageid": 3, "fullurl": "https://wiki.test/wiki/帮助:小_图"}, "https://wiki.test/wiki/帮助:小_图", false},
		{"pending_new", Object{"missing": true, "fullurl": "https://wiki.test/wiki/新页面"}, "", false},
		{"invalid_response", Object{"pageid": 3}, "", true},
		{"upstream_error", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
				if q.Get("prop") != "info" || q.Get("inprop") != "url" || q.Get("titles") != "帮助:小 图" {
					t.Fatalf("query: %v", q)
				}
				if tc.page == nil {
					return Object{"error": Object{"code": "permissiondenied"}}
				}
				return Object{"query": Object{"pages": []any{tc.page}}}
			})
			got, err := s.pageURL(context.Background(), c, PageRef{Title: "帮助:小 图"})
			if (err != nil) != tc.fail || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestEditURLPersistenceAndOutputs(t *testing.T) {
	e := Edit{ID: "edit_test", URL: "https://wiki.test/wiki/帮助:小_图", Content: "text"}
	data, err := json.Marshal(pack(e))
	if err != nil {
		t.Fatal(err)
	}
	var payload editPayload
	if err = json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	restored := unpack(payload)
	if restored.URL != e.URL || status(restored)["url"] != e.URL {
		t.Fatal("URL lost during persistence")
	}
	_, c, _ := additionalReadFixture(t, func(url.Values) Object { t.Fatal("unexpected upstream query"); return nil })
	out, err := content(restored, c, Input{})
	if err != nil || out["url"] != e.URL {
		t.Fatalf("content: %v %v", out, err)
	}
	payload = editPayload{}
	if err = json.Unmarshal([]byte(`{"Edit":{"edit_id":"legacy"}}`), &payload); err != nil {
		t.Fatal(err)
	}
	if unpack(payload).URL != "" {
		t.Fatal("legacy URL must be empty")
	}
}

func TestSnapshotURLFollowsResolvedPage(t *testing.T) {
	s, c, _ := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("inprop") != "url" || q.Get("redirects") != "1" {
			t.Fatalf("query: %v", q)
		}
		return Object{"query": Object{"redirects": []any{Object{"from": "帮助:小 图", "to": "帮助:目标"}}, "pages": []any{Object{"pageid": 3, "title": "帮助:目标", "fullurl": "https://wiki.test/wiki/帮助:目标", "revisions": []any{Object{"revid": 42, "slots": Object{"main": Object{"content": "text"}}}}}}}}
	})
	c.Config.Static.Extensions.Moderation.Enabled = false
	out, err := s.snapshot(context.Background(), c, PageRef{Title: "帮助:小 图"}, true)
	if err != nil || out["url"] != "https://wiki.test/wiki/帮助:目标" {
		t.Fatalf("snapshot: %v %v", out, err)
	}
}
