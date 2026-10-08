package wiki

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
)

// 使用真实 MediaWiki 客户端和内存 HTTP transport，验证参数编码、身份与响应解析。
func additionalReadFixture(t *testing.T, response func(url.Values) Object) (*Service, *Call, *[]url.Values) {
	t.Helper()
	requests := []url.Values{}
	cfg := config.Defaults()
	cfg.App.MediaWiki.APIURL = "https://wiki.test/api.php"
	cfg.App.MediaWiki.SiteID = "site"
	cfg.App.MediaWiki.Bridge.KeyID = "test"
	cfg.Secrets.BridgeSecret = strings.Repeat("s", 32)
	client := mediawiki.New(cfg)
	client.HTTP.Transport = pageReadTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Isekai-User-Id") != "17" || r.Header.Get("Signature") == "" {
			t.Fatal("authenticated request headers are missing")
		}
		q := r.URL.Query()
		requests = append(requests, q)
		body, err := json.Marshal(response(q))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	c := &Call{
		Principal:   &application.Principal{Issuer: "issuer", AccountID: "account", WikiUserID: 17, Scopes: []string{"wiki:read"}},
		Credentials: mediawiki.Credentials{UserID: 17, Token: "test"}, Client: client, Config: cfg,
	}
	return &Service{}, c, &requests
}

func assertReadCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || failure.Describe(err).Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestContributionIdentityAndPagination(t *testing.T) {
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		out := Object{"query": Object{"usercontribs": []any{Object{"pageid": 3, "title": "帮助:编辑/章节", "revid": 10, "parentid": 9}}}}
		if q.Get("uccontinue") == "" {
			out["continue"] = Object{"continue": "-||", "uccontinue": "20261007000000|10"}
		}
		return out
	})
	input := Input{Limit: 1, Namespaces: []int{0, 10}, Show: []string{"!minor"}, Tag: "ai-generated"}
	out, err := s.Read(context.Background(), c, "get_my_contributions", input)
	if err != nil {
		t.Fatal(err)
	}
	q := (*requests)[0]
	if q.Get("ucuserids") != "17" || q.Get("ucdir") != "older" || q.Get("ucnamespace") != "0|10" || q.Get("ucshow") != "!minor" || q.Get("uctag") != input.Tag {
		t.Fatalf("incorrect contribution query: %#v", q)
	}
	if number(out["user_id"]) != 17 || object(object(out["results"].([]any)[0])["resources"])["by_id"] != "wiki://pages/by-id/3" {
		t.Fatalf("incorrect contribution metadata: %#v", out)
	}
	input.Cursor = str(out["next_cursor"])
	if input.Cursor == "" {
		t.Fatal("missing cursor")
	}
	out, err = s.Read(context.Background(), c, "get_my_contributions", input)
	if err != nil || out["next_cursor"] != nil {
		t.Fatalf("last page: %#v %v", out, err)
	}
	if (*requests)[1].Get("continue") != "-||" || (*requests)[1].Get("uccontinue") != "20261007000000|10" {
		t.Fatal("continuation was not preserved")
	}

	before := len(*requests)
	changed := input
	changed.Tag = "other"
	_, err = s.Read(context.Background(), c, "get_my_contributions", changed)
	assertReadCode(t, err, "INVALID_CURSOR")
	other := *c
	p := *c.Principal
	p.AccountID = "other"
	other.Principal = &p
	_, err = s.Read(context.Background(), &other, "get_my_contributions", input)
	assertReadCode(t, err, "INVALID_CURSOR")
	_, err = s.Read(context.Background(), c, "get_user_contributions", Input{UserID: 17, Cursor: input.Cursor, Limit: 1})
	assertReadCode(t, err, "INVALID_CURSOR")
	_, err = s.Read(context.Background(), c, "get_my_contributions", Input{UserID: 99})
	assertReadCode(t, err, "INVALID_REQUEST")
	if len(*requests) != before {
		t.Fatal("invalid cursor reached upstream")
	}

	_, err = s.Read(context.Background(), c, "get_user_contributions", Input{Username: "测试用户"})
	if err != nil || (*requests)[before].Get("ucuser") != "测试用户" {
		t.Fatalf("username selector: %v", err)
	}
}

func TestActivityFiltersAndUpstreamFailures(t *testing.T) {
	reply := Object{"query": Object{"recentchanges": []any{}}}
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("list") == "users" {
			return Object{"query": Object{"users": []any{Object{"userid": 9, "name": "Editor"}}}}
		}
		return reply
	})
	out, err := s.Read(context.Background(), c, "get_recent_changes", Input{UserID: 9, Types: []string{"edit", "log"}, Direction: "newer", Start: "2026-10-01T00:00:00Z", End: "2026-10-08T00:00:00Z"})
	if err != nil || len(out["results"].([]any)) != 0 || out["next_cursor"] != nil {
		t.Fatalf("empty recent changes: %#v %v", out, err)
	}
	q := (*requests)[1]
	if (*requests)[0].Get("ususerids") != "9" || q.Get("rcuser") != "Editor" || q.Get("rctype") != "edit|log" || !strings.Contains(q.Get("rcprop"), "loginfo") {
		t.Fatalf("recent query: %#v", q)
	}

	for _, input := range []Input{
		{Direction: "wrong"}, {UserID: 9, Username: "A"}, {Username: "A|B"},
		{Show: []string{"minor", "!minor"}}, {Namespaces: []int{-1}}, {Types: []string{"bad"}},
		{Start: "yesterday"}, {Direction: "older", Start: "2026-10-01T00:00:00Z", End: "2026-10-08T00:00:00Z"},
	} {
		_, err = s.Read(context.Background(), c, "get_recent_changes", input)
		assertReadCode(t, err, "INVALID_REQUEST")
	}
	if len(*requests) != 2 {
		t.Fatal("invalid activity input reached upstream")
	}
	reply = Object{"query": Object{}}
	_, err = s.Read(context.Background(), c, "get_recent_changes", Input{})
	assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
	reply = Object{"query": Object{"recentchanges": []any{}}, "continue": Object{"pageids": "99"}}
	_, err = s.Read(context.Background(), c, "get_recent_changes", Input{})
	assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
	reply = Object{"error": Object{"code": "permissiondenied"}}
	_, err = s.Read(context.Background(), c, "get_recent_changes", Input{})
	assertReadCode(t, err, "PERMISSION_DENIED")
	c.Principal.Scopes = nil
	before := len(*requests)
	_, err = s.Read(context.Background(), c, "get_recent_changes", Input{})
	assertReadCode(t, err, "INSUFFICIENT_SCOPE")
	if len(*requests) != before {
		t.Fatal("unauthorized read reached upstream")
	}
}

func TestParsedSnapshotPagination(t *testing.T) {
	sections := []any{Object{"index": "1", "line": "第一节", "fromtitle": "帮助:编辑"}, Object{"index": "2", "line": "第二节"}}
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("action") == "query" {
			return Object{"query": Object{"pages": []any{Object{"pageid": 3, "title": "帮助:编辑", "actions": Object{"read": []any{}}, "revisions": []any{Object{"revid": 10, "slots": Object{"main": Object{"sha1": "visible"}}}}}}}}
		}
		return Object{"parse": Object{"pageid": 3, "title": "帮助:编辑", "revid": 10, "sections": sections}}
	})
	input := Input{Title: "Help:编辑", Limit: 1}
	out, err := s.Read(context.Background(), c, "get_page_structure", input)
	if err != nil {
		t.Fatal(err)
	}
	if (*requests)[0].Get("page") != input.Title || number(out["revision_id"]) != 10 || number(out["total"]) != 2 {
		t.Fatalf("first parsed read: %#v", out)
	}
	if object(out["resources"])["by_revision"] != "wiki://revisions/10/structure" {
		t.Fatal("missing fixed revision resource")
	}
	input.Cursor = str(out["next_cursor"])
	out, err = s.Read(context.Background(), c, "get_page_structure", input)
	if err != nil || object(out["results"].([]any)[0])["index"] != "2" || out["next_cursor"] != nil {
		t.Fatalf("parsed continuation: %#v %v", out, err)
	}
	if (*requests)[1].Get("revids") != "10" || (*requests)[1].Get("rvprop") != "ids|slotsha1" {
		t.Fatal("parsed cursor did not fix revision")
	}
	sections[1] = Object{"index": "2", "line": "模板变动后的第二节"}
	// Cached pagination preserves the selected parse until expiry.
	cached, err := s.Read(context.Background(), c, "get_page_structure", input)
	if err != nil || object(cached["results"].([]any)[0])["line"] != "第二节" {
		t.Fatalf("cached parse changed: %#v %v", cached, err)
	}
	s.pageCache.mu.Lock()
	for key, entry := range s.pageCache.entries {
		entry.expires = time.Now().Add(-time.Second)
		s.pageCache.entries[key] = entry
	}
	s.pageCache.mu.Unlock()
	_, err = s.Read(context.Background(), c, "get_page_structure", input)
	assertReadCode(t, err, "SNAPSHOT_UNAVAILABLE")
	_, err = s.Read(context.Background(), c, "get_page_structure", Input{Title: "Other", Limit: 1, Cursor: input.Cursor})
	assertReadCode(t, err, "INVALID_CURSOR")
	sections = []any{}
	out, err = s.Read(context.Background(), c, "get_page_structure", Input{RevisionID: 10})
	if err != nil || out["next_cursor"] != nil || number(out["total"]) != 0 {
		t.Fatalf("revision without sections: %#v %v", out, err)
	}
	_, err = s.Read(context.Background(), c, "get_page_structure", Input{PageID: 3, RevisionID: 10})
	assertReadCode(t, err, "INVALID_REQUEST")
}

func TestPageDependenciesAndTransclusions(t *testing.T) {
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object {
		if q.Get("list") == "embeddedin" {
			out := Object{"query": Object{"embeddedin": []any{Object{"pageid": 6, "title": "目标页面"}}}}
			if q.Get("eicontinue") == "" {
				out["continue"] = Object{"continue": "-||", "eicontinue": "6"}
			}
			return out
		}
		return Object{"parse": Object{
			"pageid": 3, "title": "模板:人物", "revid": 10,
			"templates": []any{Object{"ns": 10, "title": "模板:基础"}}, "links": []any{},
			"images": []any{"人物.png"}, "categories": []any{Object{"name": "人物", "sortkey": "A"}},
			"externallinks": []any{"https://example.test/"},
		}}
	})
	out, err := s.Read(context.Background(), c, "get_page_dependencies", Input{PageID: 3, DependencyTypes: []string{"images", "external_links"}})
	if err != nil {
		t.Fatal(err)
	}
	items := out["results"].([]any)
	if len(items) != 2 || object(items[0])["type"] != "images" || object(items[0])["data"] != "人物.png" || object(items[1])["type"] != "external_links" {
		t.Fatalf("dependencies: %#v", items)
	}
	if (*requests)[0].Get("pageid") != "3" || (*requests)[0].Get("prop") != "revid|images|externallinks" {
		t.Fatal("dependency query mismatch")
	}
	input := Input{Title: "模板:人物", Limit: 1, Namespaces: []int{0}}
	out, err = s.Read(context.Background(), c, "get_transclusions", input)
	if err != nil {
		t.Fatal(err)
	}
	input.Cursor = str(out["next_cursor"])
	out, err = s.Read(context.Background(), c, "get_transclusions", input)
	if err != nil || out["next_cursor"] != nil {
		t.Fatalf("transclusions: %#v %v", out, err)
	}
	if (*requests)[2].Get("eititle") != input.Title || (*requests)[2].Get("eicontinue") != "6" || (*requests)[2].Get("einamespace") != "0" {
		t.Fatal("transclusion query mismatch")
	}
}

func TestTemplateDataMissingAndUnavailable(t *testing.T) {
	reply := Object{"pages": Object{"3": Object{"title": "模板:人物", "params": Object{"姓名": Object{"required": true}}}}}
	s, c, requests := additionalReadFixture(t, func(q url.Values) Object { return reply })
	out, err := s.Read(context.Background(), c, "get_template_data", Input{Title: "Template:人物", Language: "zh"})
	if err != nil || out["has_template_data"] != true || object(object(out["data"])["params"])["姓名"] == nil {
		t.Fatalf("template data: %#v %v", out, err)
	}
	if (*requests)[0].Get("action") != "templatedata" || (*requests)[0].Get("titles") != "Template:人物" || (*requests)[0].Get("lang") != "zh" || (*requests)[0].Get("includeMissingTitles") != "1" {
		t.Fatal("template data query mismatch")
	}
	reply = Object{"pages": Object{"3": Object{"title": "模板:人物", "notemplatedata": true, "params": Object{"inferred": Object{}}}}}
	out, err = s.Read(context.Background(), c, "get_template_data", Input{PageID: 3})
	if err != nil || out["has_template_data"] != false || out["data"] != nil {
		t.Fatalf("missing template data: %#v %v", out, err)
	}
	reply = Object{"pages": Object{"-1": Object{"title": "模板:缺失", "missing": true}}}
	_, err = s.Read(context.Background(), c, "get_template_data", Input{Title: "模板:缺失"})
	assertReadCode(t, err, "NOT_FOUND")
	reply = Object{"error": Object{"code": "unknown_action"}}
	_, err = s.Read(context.Background(), c, "get_template_data", Input{PageID: 3})
	assertReadCode(t, err, "FEATURE_UNAVAILABLE")
	reply = Object{"error": Object{"code": "templatedata-corrupt"}}
	_, err = s.Read(context.Background(), c, "get_template_data", Input{PageID: 3})
	assertReadCode(t, err, "UPSTREAM_ERROR")
	reply = Object{"pages": []any{}}
	_, err = s.Read(context.Background(), c, "get_template_data", Input{PageID: 3})
	assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
	reply = Object{"pages": Object{"3": Object{"title": "模板:人物"}}}
	_, err = s.Read(context.Background(), c, "get_template_data", Input{PageID: 3})
	assertReadCode(t, err, "UPSTREAM_RESPONSE_INVALID")
}
