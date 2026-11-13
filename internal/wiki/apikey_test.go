package wiki_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
"strings"
	"testing"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/auth/repository/mysql"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
	"isekai-mcp-gateway/internal/wiki"
)

// wikiAPIKey 分支使用真实 MySQL 仓库与 Bridge 行为，未配置环境时跳过。
func TestAPIKeyCredentialsBranch(t *testing.T) {
	store, cfg := scratchStore(t)
	ctx := context.Background()
	calls := 0
	var tokens []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		tokens = append(tokens, r.Header.Get("X-Isekai-User-Token"))
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("method") == "oidcgetuserinfo" {
			// Bridge 身份解析：返回新 token 的完整身份。
			_, _ = w.Write([]byte(`{"gatewayservice":{"userinfo":{"user_id":7,"username":"alice","display_name":"Alice","user_token":"refreshed-token"}}}`))
			return
		}
		if r.Form.Get("meta") == "userinfo" && r.Header.Get("X-Isekai-User-Token") == "stale-token" {
			// 旧 token 的轻量探测被 Bridge 拒绝，触发刷新。
			calls++
			_, _ = w.Write([]byte(`{"error":{"code":"isekai-servicebridge-oidc_user_unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"userinfo":{}}}`))
	}))
	defer upstream.Close()
	cfg.App.MediaWiki.APIURL = upstream.URL
	cfg.App.MediaWiki.SiteID = "test-wiki"
	cfg.App.MediaWiki.Bridge.KeyID = "test-key"
	cfg.Secrets.BridgeSecret = strings.Repeat("s", 32)
	cfg.Static.Server.PublicURL = "https://gateway.test"
	config.Publish(cfg)
	defer config.Publish(nil)

	exp := time.Now().Add(time.Hour)
	keyID := security.Random()[:16]
	key := domain.APIKey{ID: keyID, AccountID: "account", Name: "desktop", Scopes: []string{"mcp:access", "wiki:read"}, Identity: domain.Identity{UserID: 7, Username: "alice", UserToken: "stale-token"}, SecretHash: "hash"}
	if e := store.Create(store.DB, "api_keys", keyID, key, persistence.Row{AccountID: "account", State: "active", ExpiresAt: exp}); e != nil {
		t.Fatal(e)
	}
	if e := store.Create(store.DB, "accounts", "account", domain.Account{ID: "account", SiteID: "test-wiki", WikiUserID: 7}, persistence.Row{State: "active", ExpiresAt: exp}); e != nil {
		t.Fatal(e)
	}
	manager := new(outbound.Manager)
	manager.Publish(&outbound.Bundle{Config: cfg, Bridge: mediawiki.New(cfg)})
	authService := &application.Service{Config: cfg, Store: mysql.New(store), Upstream: manager}
	service := &wiki.Service{Auth: authService, Repo: &wiki.Repository{Store: store}}
	principal := &application.Principal{Issuer: "https://gateway.test", AccountID: "account", OAuthClientID: "api-key", GrantID: keyID, WikiUserID: 7, Scopes: []string{"mcp:access", "wiki:read"}, ExpiresAt: exp, AuthKind: domain.AuthKindAPIKey}
	extra := &mcp.RequestExtra{TokenInfo: &sdkauth.TokenInfo{Extra: map[string]any{"principal": principal}}}
	call, e := service.Call(ctx, extra, "wiki:read")
	if e != nil {
		t.Fatal(e)
	}
	// 凭据来自 api_keys 快照，且失效后已刷新重试。
	if call.Credentials.Token != "refreshed-token" || call.Credentials.UserID != 7 {
		t.Fatalf("credentials: %+v", call.Credentials)
	}
	if calls != 1 {
		t.Fatalf("expected one rejected probe, got %d", calls)
	}
	// 刷新后的快照已回写。
	var refreshed domain.APIKey
	row, e := store.Get(store.DB, "api_keys", keyID, &refreshed, false)
	if e != nil {
		t.Fatal(e)
	}
	var plain struct {
		Identity struct {
			UserToken string `json:"user_token"`
		} `json:"Identity"`
	}
	_ = json.Unmarshal(row.Payload, &plain)
	if refreshed.Identity.UserID != 7 || plain.Identity.UserToken != "" || !strings.Contains(string(row.EncryptedPayload), "refreshed-token") {
		t.Fatalf("refreshed snapshot not sealed: %+v", refreshed)
	}
}


