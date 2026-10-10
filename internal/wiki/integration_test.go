package wiki_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mysqlDriver "github.com/go-sql-driver/mysql"
	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	mysqlrepo "isekai-mcp-gateway/internal/auth/repository/mysql"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/help"
	"isekai-mcp-gateway/internal/mcp/modules/wikiactivity"
	"isekai-mcp-gateway/internal/mcp/modules/wikiedit"
	"isekai-mcp-gateway/internal/mcp/modules/wikimutation"
	"isekai-mcp-gateway/internal/mcp/modules/wikiread"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
	"isekai-mcp-gateway/internal/templates"
	"isekai-mcp-gateway/internal/wiki"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func scratchStore(t *testing.T) (*persistence.Store, *config.Config) {
	t.Helper()
	// 未配置测试目录时跳过 MySQL 集成测试。
	dir := os.Getenv("GATEWAY_WIKI_TEST_CONFIG")
	if dir == "" {
		t.Skip("set GATEWAY_WIKI_TEST_CONFIG to a configuration directory to run isolated MySQL integration tests")
	}
	cfg := config.Defaults()
	if e := config.Decode(filepath.Join(dir, "server.json"), &cfg.Static); e != nil {
		t.Fatal(e)
	}
	d := mysqlDriver.NewConfig()
	d.User = cfg.Static.Storage.Username
	d.Passwd = cfg.Static.Storage.Password
	if d.Passwd == "" {
		d.Passwd = os.Getenv(cfg.Static.Storage.PasswordEnv)
	}
	d.Net = "tcp"
	d.Addr = fmt.Sprintf("%s:%d", cfg.Static.Storage.Host, cfg.Static.Storage.Port)
	admin, e := sql.Open("mysql", d.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	// 每次运行创建独立数据库，测试结束后销毁。
	name := "gateway_wiki_test_" + security.Random()[:16]
	if _, e = admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); e != nil {
		t.Fatal("create isolated database:", e)
	}
	t.Cleanup(func() {
		// 校验库名前缀后再删除，防止误删生产库。
		if !strings.HasPrefix(name, "gateway_wiki_test_") {
			panic("invalid scratch database")
		}
		_, _ = admin.Exec("DROP DATABASE `" + name + "`")
		_ = admin.Close()
	})
	cfg.Static.Storage.Database = name
	cfg.Secrets.DBPassword = d.Passwd
	cfg.Secrets.Storage = []byte(strings.Repeat("t", 32))
	cfg.App.Uploads.TempPath = t.TempDir()
	store, e := persistence.Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = store.SQL.Close() })
	if e = store.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}

	// 迁移执行两次以验证幂等。
	if e = store.Migrate(context.Background()); e != nil {
		t.Fatal("idempotent migration:", e)
	}
	if e = store.CheckSchema(); e != nil {
		t.Fatal(e)
	}
	return store, cfg
}
func TestMySQLWikiIntegration(t *testing.T) {
	store, cfg := scratchStore(t)
	ctx := context.Background()
	var mode atomic.Int32
	var failURL atomic.Bool
	var deny atomic.Bool
	var writes atomic.Int32
	var wikiQueries, fullContentQueries atomic.Int32
	var uploadWrites atomic.Int32
	var uploadMode atomic.Int32
	var blockCommit atomic.Bool
	commitStarted := make(chan struct{}, 1)
	commitRelease := make(chan struct{})
	var revision atomic.Int64
	revision.Store(10)
	var upstreamMu sync.Mutex
	upstreamText := "a\nb\nc"
	warning := true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		q := r.Form
		// multipart 请求需要单独解析表单。
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			if e := r.ParseMultipartForm(8 << 20); e != nil {
				t.Error(e)
			}
			defer r.MultipartForm.RemoveAll()
			q = r.Form
		}
		var v any
		switch q.Get("action") {
		case "edit":
			writes.Add(1)

			// 阻塞提交，用于构造关闭与提交并发执行的场景。
			if blockCommit.Load() {
				commitStarted <- struct{}{}
				<-commitRelease
			}

			// mode 控制上游返回，模拟冲突、断网、审核排队等分支。
			switch mode.Load() {
			case 1:
				v = wiki.Object{"error": wiki.Object{"code": "editconflict"}}
			case 2:
				// 直接断开连接，模拟网络中断。
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
				return
			case 3:
				v = wiki.Object{"error": wiki.Object{"code": "moderation-edit-queued", "moderation": wiki.Object{"mod_id": 1, "temp_revision_id": 2, "chain_version": 3}}}
			case 4:
				v = wiki.Object{"edit": wiki.Object{"result": "Success", "nochange": true, "pageid": 1, "title": "Page"}}
			default:
				revision.Add(1)
				upstreamMu.Lock()
				upstreamText = q.Get("text")
				upstreamMu.Unlock()
				v = wiki.Object{"edit": wiki.Object{"result": "Success", "pageid": 1, "title": "Page", "newrevid": revision.Load()}}
			}
		case "upload":
			uploadWrites.Add(1)
			if uploadMode.Load() == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
				return
			}
			if r.Header.Get("X-Isekai-Multipart-Digest") == "" || !strings.Contains(r.Header.Get("Signature-Input"), "x-isekai-multipart-digest") {
				t.Error("multipart signature missing")
			}
			if warning && q.Get("ignorewarnings") == "" {
				v = wiki.Object{"upload": wiki.Object{"result": "Warning", "warnings": wiki.Object{"duplicate": "existing"}}}
			} else {
				v = wiki.Object{"upload": wiki.Object{"result": "Success", "filename": "test.pdf", "imageinfo": wiki.Object{"url": "https://example.test/test.pdf"}}}
			}
		case "parse":
			v = wiki.Object{"parse": wiki.Object{"text": "<p>preview</p>"}}
		case "query":
			wikiQueries.Add(1)
			if strings.Contains(q.Get("rvprop"), "content") {
				fullContentQueries.Add(1)
			}
			// CSRF token 换取分支。
			if q.Get("meta") == "tokens" {
				v = wiki.Object{"query": wiki.Object{"tokens": wiki.Object{"csrftoken": "csrf"}}}
				break
			}

			// search 分支返回 continue，用于测试游标签发。
			if q.Get("list") == "search" {
				v = wiki.Object{"query": wiki.Object{"search": []any{wiki.Object{"pageid": 1, "title": "Page"}}}, "continue": wiki.Object{"continue": "-||", "sroffset": 1}}
				break
			}
			title := q.Get("titles")
			if title == "" {
				title = "Page"
			}
			if q.Get("prop") == "info" && q.Get("inprop") == "url" && failURL.Load() {
				_ = json.NewEncoder(w).Encode(wiki.Object{"error": wiki.Object{"code": "permissiondenied"}})
				return
			}
			page := wiki.Object{"pageid": 1, "title": title, "readable": true, "fullurl": "https://wiki.test/wiki/" + title}
			missing := strings.HasPrefix(title, "File:") || strings.HasPrefix(title, "New")
			if missing {
				page["pageid"] = -1
				page["missing"] = true
			}
			// 按 deny 开关模拟编辑权限被拒。
			if q.Get("intestactions") != "" {
				actions := wiki.Object{"edit": []any{}, "create": []any{}, "read": []any{}}
				if deny.Load() {
					actions["edit"] = []any{wiki.Object{"code": "permissiondenied"}}
				}
				page["actions"] = actions
			}
			upstreamMu.Lock()
			text := upstreamText
			upstreamMu.Unlock()
			if strings.Contains(q.Get("prop"), "revisions") {
				if missing {
					page["revisions"] = []any{}
				} else {
					page["revisions"] = []any{wiki.Object{"revid": revision.Load(), "timestamp": "2026-10-07T00:00:00Z", "slots": wiki.Object{"main": wiki.Object{"content": text, "contentmodel": "wikitext", "sha1": "visible"}}}}
				}
			}
			if strings.Contains(q.Get("prop"), "isekaiuseredited") {
				page["isekaiuseredited"] = wiki.Object{"has_edited": 1}
			}
			// 构造 Moderation 审核状态响应，missing 时为空内容源。
			if strings.Contains(q.Get("prop"), "moderationeditstate") {
				source := "baseline"
				id, pub := any(int64(1)), any(revision.Load())
				if missing {
					source = "empty"
					id = nil
					pub = nil
					text = ""
				}
				page["moderationeditstate"] = wiki.Object{"page_id": id, "title": title, "public_revision_id": pub, "content_source": source, "content": text, "content_hash": security.Hash(text), "temp_revision_id": nil, "chain_version": nil}
			}
			v = wiki.Object{"query": wiki.Object{"pages": []any{page}}}
		default:
			v = wiki.Object{"error": wiki.Object{"code": "unknown_action"}}
		}
		_ = json.NewEncoder(w).Encode(v)
	}))
	defer upstream.Close()
	cfg.App.MediaWiki.APIURL = upstream.URL
	cfg.App.MediaWiki.SiteID = "test-wiki"
	cfg.App.MediaWiki.Bridge.KeyID = "test-key"
	cfg.Secrets.BridgeSecret = strings.Repeat("s", 32)
	cfg.Static.Extensions.Moderation.Enabled = false
	cfg.Static.Extensions.ACL.Enabled = false
	cfg.Static.Server.PublicURL = "https://gateway.test"
	config.Publish(cfg)
	defer config.Publish(nil)
	principal := &application.Principal{Issuer: "https://gateway.test", AccountID: "account", OAuthClientID: "client", GrantID: "grant", WikiUserID: 1, Scopes: application.BusinessScopes(), ExpiresAt: time.Now().Add(time.Hour)}
	exp := time.Now().Add(time.Hour)
	records := []struct {
		kind, key string
		value     any
	}{
		{"clients", "client", domain.StoredClient{Metadata: domain.Client{ID: "client", Name: "Test", Scope: strings.Join(principal.Scopes, " ")}, Fingerprint: "fingerprint"}},
		{"accounts", "account", domain.Account{ID: "account", SiteID: "test-wiki", WikiUserID: 1}},
		{"sessions", "session", domain.Session{ID: "session", AccountID: "account", Identity: domain.Identity{UserID: 1, UserToken: "private-token"}}},
		{"grants", "grant", domain.Grant{ID: "grant", AccountID: "account", ClientID: "client", SessionID: "session", Fingerprint: "fingerprint", Scopes: principal.Scopes}},
	}
	// 写入 principal 对应的客户端、账户、会话与授权记录。
	for _, r := range records {
		row := persistence.Row{State: "active", ExpiresAt: exp}
		if client, ok := r.value.(domain.StoredClient); ok {
			row.Fingerprint = client.Fingerprint
			row.ClientName = client.Metadata.Name
			row.TrustLevel = "unknown"
		}
		if e := store.Create(store.DB, r.kind, r.key, r.value, row); e != nil {
			t.Fatal(e)
		}
	}
	manager := new(outbound.Manager)
	manager.Publish(&outbound.Bundle{Config: cfg, Bridge: mediawiki.New(cfg)})
	authService := &application.Service{Config: cfg, Store: mysqlrepo.New(store), Upstream: manager}
	service := &wiki.Service{Auth: authService, Repo: &wiki.Repository{Store: store}}
	extra := &mcp.RequestExtra{TokenInfo: &sdkauth.TokenInfo{Extra: map[string]any{"principal": principal}}}
	call, e := service.Call(ctx, extra, "wiki:read")
	if e != nil {
		t.Fatal(e)
	}
	renderer, e := templates.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defs := help.Module(&gatewaymcp.Authenticator{Service: authService}, renderer)
	defs = append(defs, wikiread.Module(service)...)
	defs = append(defs, wikiactivity.Module(service)...)
	defs = append(defs, wikiedit.Module(service)...)
	defs = append(defs, wikimutation.Module(service)...)
	server := mcp.NewServer(&mcp.Implementation{Name: "wiki-test", Version: "1"}, nil)
	if e = gatewaymcp.Register(server, defs); e != nil {
		t.Fatal(e)
	}
	// 中间件为每个入站请求注入测试 token。
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
			if x := r.GetExtra(); x != nil {
				x.TokenInfo = extra.TokenInfo
			}
			return next(ctx, method, r)
		}
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{DisableLocalhostProtection: true}))
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpServer.Client(), DisableStandaloneSSE: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	tool := func(name string, input any) wiki.Object {
		t.Helper()
		r, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "wiki." + name, Arguments: input})
		if e != nil || r.IsError {
			t.Fatalf("tool %s: %+v %v", name, r, e)
		}
		b, _ := json.Marshal(r.StructuredContent)
		var out wiki.Object
		_ = json.Unmarshal(b, &out)
		return out
	}
	var editID string
	var version int64
	t.Run("protocol_registration_and_schemas", func(t *testing.T) {
		list, e := session.ListTools(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		if len(list.Tools) != 35 {
			t.Fatalf("got %d tools", len(list.Tools))
		}
		for _, tool := range list.Tools {
			if tool.InputSchema == nil || tool.OutputSchema == nil {
				t.Fatalf("missing schema %s", tool.Name)
			}
		}
		if _, e = session.CallTool(ctx, &mcp.CallToolParams{Name: "wiki.begin_edit", Arguments: wiki.Object{"page": wiki.Object{"page_id": 1, "title": "Page"}}}); e == nil {
			t.Fatal("invalid page schema accepted")
		}
		ts, e := session.ListResourceTemplates(ctx, nil)
		if e != nil || len(ts.ResourceTemplates) != 26 {
			t.Fatalf("templates %+v %v", ts, e)
		}
	})
	t.Run("edit_concurrency_and_resources", func(t *testing.T) {
		beforeQueries := wikiQueries.Load()
		out := tool("begin_edit", wiki.Object{"page": wiki.Object{"page_id": 1}})
		if wikiQueries.Load() != beforeQueries+1 {
			t.Fatal("begin_edit did not merge content and permission queries")
		}
		editID = out["edit_id"].(string)
		version = int64(out["version"].(float64))
		var successes atomic.Int32
		var wg sync.WaitGroup

		// 两个并发更新只能有一个成功，另一个应报版本冲突。
		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				_, e := service.Update(ctx, call, wiki.Input{EditID: editID, BaseVersion: version, Operations: []wiki.Operation{{Type: "replace_content", Content: fmt.Sprint("updated ", k)}}})
				if e == nil {
					successes.Add(1)
				} else if failure.Describe(e).Code != "EDIT_SESSION_VERSION_MISMATCH" {
					t.Error(e)
				}
			}(k)
		}
		wg.Wait()
		if successes.Load() != 1 {
			t.Fatal("version updates did not serialize")
		}
		status := tool("get_edit_status", wiki.Object{"edit_id": editID})
		version = int64(status["version"].(float64))
		r, e := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "wiki://edit/" + editID + "/content"})
		if e != nil || len(r.Contents) != 1 || strings.Contains(r.Contents[0].Text, "1 | ") {
			t.Fatal(r, e)
		}
		list, e := session.ListResources(ctx, nil)
		if e != nil || len(list.Resources) != 5 {
			t.Fatal(list, e)
		}
		copyCall := *call
		p := *principal
		p.AccountID = "other"
		copyCall.Principal = &p
		if _, e = service.Get(ctx, &copyCall, "get_edit_content", wiki.Input{EditID: editID}); failure.Describe(e).Code != "NOT_FOUND" {
			t.Fatal("cross owner read", e)
		}
	})
	t.Run("save_nochange_and_replay", func(t *testing.T) {
		beforeContent := fullContentQueries.Load()
		if _, err := service.Get(ctx, call, "get_edit_diff", wiki.Input{EditID: editID, Comparison: "current_to_edited"}); err != nil {
			t.Fatal(err)
		}
		if fullContentQueries.Load() != beforeContent+1 {
			t.Fatal("diff downloaded the public baseline twice")
		}
		first, err := service.Read(ctx, call, "get_latest_page", wiki.Input{Title: "Page", Format: "html", Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		continuation := wiki.Input{Title: "Page", Format: "html", Limit: 4, Cursor: first["next_cursor"].(string)}
		beforeContent = fullContentQueries.Load()
		if _, err := service.Read(ctx, call, "get_latest_page", continuation); err != nil {
			t.Fatal(err)
		}
		if fullContentQueries.Load() != beforeContent {
			t.Fatal("continuation did not use the cache")
		}
		before := writes.Load()
		out := tool("commit_edit", wiki.Object{"edit_id": editID, "base_version": version, "summary": "test", "ai_involvement": "none"})
		if out["outcome"] != "saved" || out["url"] != "https://wiki.test/wiki/Page" {
			t.Fatal(out)
		}
		// This upstream fixture changes its latest revision on save. The old cursor
		// must attempt an upstream reload rather than serve the pre-save cache.
		_, _ = service.Read(ctx, call, "get_latest_page", continuation)
		if fullContentQueries.Load() != beforeContent+1 {
			t.Fatal("commit did not invalidate the page cache")
		}

		// 重放同一提交应命中缓存，不再写入上游。
		out = tool("commit_edit", wiki.Object{"edit_id": editID, "base_version": version, "summary": "test", "ai_involvement": "none"})
		if writes.Load() != before+1 {
			t.Fatal("cached result issued another save")
		}
		version = int64(out["version"].(float64))
		mode.Store(4)
		out = tool("commit_edit", wiki.Object{"edit_id": editID, "base_version": version, "summary": "same", "ai_involvement": "none"})
		if out["outcome"] != "nochange" || out["url"] != "https://wiki.test/wiki/Page" {
			t.Fatal(out)
		}
		version = int64(out["version"].(float64))
		mode.Store(0)
		status := tool("get_edit_status", wiki.Object{"edit_id": editID})
		if status["state"] != "ready" || status["has_changes"] != false {
			t.Fatal(status)
		}
	})
	t.Run("url_refresh_failure_preserves_submission", func(t *testing.T) {
		failURL.Store(true)
		before := writes.Load()
		submittedVersion := version
		out := tool("commit_edit", wiki.Object{"edit_id": editID, "base_version": version, "summary": "url failure", "ai_involvement": "none"})
		if out["outcome"] != "saved" || out["url"] != "https://wiki.test/wiki/Page" || out["url_refresh_warning"] == nil {
			t.Fatal(out)
		}
		replay := tool("commit_edit", wiki.Object{"edit_id": editID, "base_version": submittedVersion, "summary": "url failure", "ai_involvement": "none"})
		if replay["url_refresh_warning"] == nil || writes.Load() != before+1 {
			t.Fatal("URL warning replay resent write", replay)
		}
		failURL.Store(false)
		version = int64(out["version"].(float64))
		pulled := tool("pull_edit", wiki.Object{"edit_id": editID, "base_version": version})
		if pulled["url"] != "https://wiki.test/wiki/Page" {
			t.Fatal(pulled)
		}
		version = int64(pulled["version"].(float64))
	})
	t.Run("failed_submit_pull_and_network_recovery", func(t *testing.T) {
		out := tool("update_edit", wiki.Object{"edit_id": editID, "base_version": version, "operations": []any{wiki.Object{"type": "replace_content", "content": "draft"}}})
		version = int64(out["version"].(float64))

		// 模拟编辑冲突，使提交转入 unsync 状态。
		mode.Store(1)
		_, e := service.Commit(ctx, call, wiki.Input{EditID: editID, BaseVersion: version, Summary: "conflict", AIInvolvement: "none"})
		if failure.Describe(e).Code != "EDIT_CONFLICT" {
			t.Fatal(e)
		}
		out = tool("get_edit_status", wiki.Object{"edit_id": editID})
		if out["state"] != "unsync" {
			t.Fatal(out)
		}
		out = tool("pull_edit", wiki.Object{"edit_id": editID, "base_version": version})
		version = int64(out["version"].(float64))
		if out["conflict_count"].(float64) == 0 {
			t.Fatal("pull omitted conflicts")
		}
		out = tool("update_edit", wiki.Object{"edit_id": editID, "base_version": version, "operations": []any{wiki.Object{"type": "replace_content", "content": "resolved"}}})
		version = int64(out["version"].(float64))

		// 模拟网络中断：写入已发出但结果未知。
		mode.Store(2)
		before := writes.Load()
		_, e = service.Commit(ctx, call, wiki.Input{EditID: editID, BaseVersion: version, Summary: "network", AIInvolvement: "none"})
		if failure.Describe(e).Code != "API_NETWORK_ERROR" || writes.Load() != before+1 {
			t.Fatal(e)
		}
		out = tool("get_edit_status", wiki.Object{"edit_id": editID})
		if out["state"] != "unsync" {
			t.Fatal(out)
		}
		mode.Store(0)
	})
	t.Run("permissions_do_not_allocate", func(t *testing.T) {
		deny.Store(true)
		defer deny.Store(false)
		var before, after int64

		// 记录会话数量，验证权限拒绝时不分配资源。
		store.DB.Table("edit_sessions").Count(&before)
		_, e := service.Begin(ctx, call, "begin_create", wiki.Input{Title: "New denied"})
		store.DB.Table("edit_sessions").Count(&after)
		if failure.Describe(e).Code != "PERMISSION_DENIED" || after != before {
			t.Fatal("allocated denied session", e)
		}
	})
	t.Run("moderation_queued_closes", func(t *testing.T) {
		cfg2 := *cfg
		cfg2.Static.Extensions.Moderation.Enabled = true
		call2 := *call
		call2.Config = &cfg2
		mode.Store(3)
		defer mode.Store(0)
		out, e := service.Begin(ctx, &call2, "begin_edit", wiki.Input{Page: wiki.PageRef{PageID: 1}})
		if e != nil {
			t.Fatal(e)
		}
		id := out["edit_id"].(string)
		out, e = service.Commit(ctx, &call2, wiki.Input{EditID: id, BaseVersion: 1, Summary: "pending", AIInvolvement: "none"})
		if e != nil || out["outcome"] != "submitted_for_review" || out["url"] != "https://wiki.test/wiki/Page" {
			t.Fatal(out, e)
		}
		_, e = service.Get(ctx, &call2, "get_edit_status", wiki.Input{EditID: id})
		if failure.Describe(e).Code != "EDIT_SESSION_CLOSED" {
			t.Fatal(e)
		}
	})
	t.Run("new_page_url_public_and_pending", func(t *testing.T) {
		for _, pending := range []bool{false, true} {
			cfg2 := *cfg
			cfg2.Static.Extensions.Moderation.Enabled = pending
			call2 := *call
			call2.Config = &cfg2
			out, err := service.Begin(ctx, &call2, "begin_create", wiki.Input{Title: "New URL"})
			if err != nil || out["url"] != "" {
				t.Fatal(out, err)
			}
			id := out["edit_id"].(string)
			if pending {
				mode.Store(3)
			} else {
				mode.Store(0)
			}
			result, err := service.Commit(ctx, &call2, wiki.Input{EditID: id, BaseVersion: 1, Summary: "new", AIInvolvement: "none"})
			if err != nil {
				t.Fatal(err)
			}
			want := "https://wiki.test/wiki/Page"
			if pending {
				want = ""
			}
			if result["url"] != want {
				t.Fatal(result)
			}
			if !pending {
				saved, err := service.Get(ctx, &call2, "get_edit_status", wiki.Input{EditID: id})
				if err != nil || saved["url"] != want {
					t.Fatal(saved, err)
				}
			}
		}
		mode.Store(0)
	})
	t.Run("upload_receive_warning_confirm", func(t *testing.T) {
		file := []byte("%PDF-1.4\n%test\n")
		h := sha256.Sum256(file)
		out, e := service.PrepareUpload(ctx, call, wiki.Input{Filename: "test.pdf", MIMEType: "application/pdf", SizeBytes: int64(len(file)), SHA256: hex.EncodeToString(h[:]), Comment: "test"})
		if e != nil {
			t.Fatal(e)
		}
		id := out["upload_id"].(string)
		receive := func() int {
			r := httptest.NewRequest("PUT", "https://gateway.test/uploads?task_id="+id, bytes.NewReader(file))
			r.Header.Set("Content-Type", "application/octet-stream")
			w := httptest.NewRecorder()
			service.Receive(w, r)
			return w.Code
		}
		if receive() != 201 || receive() != 200 {
			t.Fatal("upload receiving failed")
		}
		_, e = service.CommitUpload(ctx, call, wiki.Input{UploadID: id})
		if failure.Describe(e).Code != "UPLOAD_WARNINGS" {
			t.Fatal(e)
		}
		before := uploadWrites.Load()
		out, e = service.CommitUpload(ctx, call, wiki.Input{UploadID: id, ConfirmedWarnings: []string{"duplicate"}})
		if e != nil || out["state"] != "completed" || uploadWrites.Load() != before+1 {
			t.Fatal(out, e)
		}

		// 成功后重复提交应命中缓存，不再上传。
		_, e = service.CommitUpload(ctx, call, wiki.Input{UploadID: id})
		if e != nil || uploadWrites.Load() != before+1 {
			t.Fatal("upload result was not cached", e)
		}
	})
	t.Run("close_confirmation_is_single_use", func(t *testing.T) {
		out, e := service.Begin(ctx, call, "begin_edit", wiki.Input{Page: wiki.PageRef{PageID: 1}})
		if e != nil {
			t.Fatal(e)
		}
		id := out["edit_id"].(string)
		out, e = service.Update(ctx, call, wiki.Input{EditID: id, BaseVersion: 1, Operations: []wiki.Operation{{Type: "replace_content", Content: "changed for closure"}}})
		if e != nil {
			t.Fatal(e)
		}
		v := out["version"].(int64)
		_, e = service.Close(ctx, call, wiki.Input{EditID: id, BaseVersion: v})
		f := failure.Describe(e)
		if f.Code != "POLICY_CHALLENGE" {
			t.Fatal(e)
		}
		challenge := f.Details["challenge_id"].(string)
		state, e := service.Get(ctx, call, "get_edit_status", wiki.Input{EditID: id})
		if e != nil || state["close_requested"] != false {
			t.Fatal("first challenge requested closure", state, e)
		}
		var successes atomic.Int32
		var wg sync.WaitGroup

		// 并发使用同一挑战，只有一次应成功。
		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := service.Close(ctx, call, wiki.Input{EditID: id, BaseVersion: v, ChallengeResponses: []wiki.Response{{ChallengeID: challenge, Confirmed: true}}})
				if e == nil {
					successes.Add(1)
				}
			}()
		}
		wg.Wait()
		if successes.Load() != 1 {
			t.Fatalf("confirmation used %d times", successes.Load())
		}
	})
	t.Run("close_during_commit_preserves_result", func(t *testing.T) {
		out, e := service.Begin(ctx, call, "begin_edit", wiki.Input{Page: wiki.PageRef{PageID: 1}})
		if e != nil {
			t.Fatal(e)
		}
		id := out["edit_id"].(string)
		out, e = service.Update(ctx, call, wiki.Input{EditID: id, BaseVersion: 1, Operations: []wiki.Operation{{Type: "replace_content", Content: "changed while committing"}}})
		if e != nil {
			t.Fatal(e)
		}
		v := out["version"].(int64)

		// 阻塞提交，制造关闭与提交并发的竞态窗口。
		blockCommit.Store(true)
		defer blockCommit.Store(false)
		done := make(chan error, 1)
		go func() {
			_, e := service.Commit(ctx, call, wiki.Input{EditID: id, BaseVersion: v, Summary: "close race", AIInvolvement: "none"})
			done <- e
		}()
		select {
		case <-commitStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("commit did not start")
		}
		_, e = service.Close(ctx, call, wiki.Input{EditID: id, BaseVersion: v})
		f := failure.Describe(e)
		if f.Code != "POLICY_CHALLENGE" {
			t.Fatal(e)
		}
		state, e := service.Get(ctx, call, "get_edit_status", wiki.Input{EditID: id})
		if e != nil || state["state"] != "loading" || state["close_requested"] != false {
			t.Fatal(state, e)
		}
		_, e = service.Close(ctx, call, wiki.Input{EditID: id, BaseVersion: v, ChallengeResponses: []wiki.Response{{ChallengeID: f.Details["challenge_id"].(string), Confirmed: true}}})
		close(commitRelease)
		if e != nil {
			t.Fatal(e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if _, e = service.Get(ctx, call, "get_edit_status", wiki.Input{EditID: id}); failure.Describe(e).Code != "EDIT_SESSION_CLOSED" {
			t.Fatal(e)
		}
		if _, e = service.Commit(ctx, call, wiki.Input{EditID: id, BaseVersion: v, Summary: "close race", AIInvolvement: "none"}); e != nil {
			t.Fatal("closed save result unavailable", e)
		}
	})
	t.Run("restart_classification", func(t *testing.T) {
		// 覆盖已发送/未发送与结果已知/未知的恢复分类。
		for _, scenario := range []struct {
			sent   bool
			reason string
			known  bool
		}{{false, "save_failed", false}, {true, "result_unknown", false}, {true, "saved_sync_failed", true}} {
			out, e := service.Begin(ctx, call, "begin_edit", wiki.Input{Page: wiki.PageRef{PageID: 1}})
			if e != nil {
				t.Fatal(e)
			}
			id := out["edit_id"].(string)
			var row wiki.Record
			if e = store.DB.Table("edit_sessions").Where("id = ?", id).Take(&row).Error; e != nil {
				t.Fatal(e)
			}
			var payload wiki.Object
			if e = store.Vault.Open(row.Payload, &payload, "edit_sessions:"+id); e != nil {
				t.Fatal(e)
			}
			payload["Edit"].(map[string]any)["state"] = "loading"
			payload["PreviousState"] = "ready"
			payload["Operation"] = "commit"
			row.State = "loading"
			row.Payload, e = store.Vault.Seal(payload, "edit_sessions:"+id)
			if e != nil {
				t.Fatal(e)
			}
			if e = store.DB.Table("edit_sessions").Where("id = ?", id).Updates(&row).Error; e != nil {
				t.Fatal(e)
			}
			a := wiki.Attempt{EditID: id, Version: 1, Sent: scenario.sent}
			if scenario.known {
				a.Result = wiki.Object{"outcome": "saved"}
			}
			ar := wiki.Record{ID: id + ":1", Owner: row.Owner, State: "sending", Version: 1, ExpiresAt: row.ExpiresAt, UpdatedAt: time.Now()}
			ar.Payload, e = store.Vault.Seal(a, "commit_attempts:"+ar.ID)
			if e != nil {
				t.Fatal(e)
			}
			if e = store.DB.Table("commit_attempts").Create(&ar).Error; e != nil {
				t.Fatal(e)
			}
			before := writes.Load()
			if e = service.Recover(ctx); e != nil {
				t.Fatal(e)
			}
			state, e := service.Get(ctx, call, "get_edit_status", wiki.Input{EditID: id})
			if e != nil || state["state"] != "unsync" || state["error"].(map[string]any)["errorCode"] != scenario.reason || writes.Load() != before {
				t.Fatal(state, e)
			}
		}
	})
	t.Run("unknown_upload_cannot_publish_again", func(t *testing.T) {
		file := []byte("%PDF-1.4\n%unknown\n")
		h := sha256.Sum256(file)
		out, e := service.PrepareUpload(ctx, call, wiki.Input{Filename: "unknown.pdf", MIMEType: "application/pdf", SizeBytes: int64(len(file)), SHA256: hex.EncodeToString(h[:])})
		if e != nil {
			t.Fatal(e)
		}
		id := out["upload_id"].(string)
		request := httptest.NewRequest("PUT", "https://gateway.test/uploads?task_id="+id, bytes.NewReader(file))
		request.Header.Set("Content-Type", "application/octet-stream")
		response := httptest.NewRecorder()
		service.Receive(response, request)
		if response.Code != 201 {
			t.Fatal(response.Body.String())
		}
		uploadMode.Store(1)
		defer uploadMode.Store(0)
		before := uploadWrites.Load()
		_, e = service.CommitUpload(ctx, call, wiki.Input{UploadID: id})
		if failure.Describe(e).Code != "MUTATION_RESULT_UNKNOWN" {
			t.Fatal(e)
		}

		// 恢复后结果未知的上传不得再次发布。
		if e = service.Recover(ctx); e != nil {
			t.Fatal(e)
		}
		_, e = service.CommitUpload(ctx, call, wiki.Input{UploadID: id})
		if failure.Describe(e).Code != "MUTATION_RESULT_UNKNOWN" || uploadWrites.Load() != before+1 {
			t.Fatal("unknown upload republished", e)
		}
	})
}
