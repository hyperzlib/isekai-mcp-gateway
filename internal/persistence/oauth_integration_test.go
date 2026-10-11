package persistence_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	mysqlrepo "isekai-mcp-gateway/internal/auth/repository/mysql"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

func TestMySQLOAuthIntegration(t *testing.T) {
	dir := os.Getenv("GATEWAY_OAUTH_TEST_CONFIG")
	if dir == "" {
		t.Skip("set GATEWAY_OAUTH_TEST_CONFIG for isolated MySQL test")
	}
	cfg, err := config.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := driver.NewConfig()
	d.User = cfg.Static.Storage.Username
	d.Passwd = cfg.Secrets.DBPassword
	d.Net = "tcp"
	d.Addr = net.JoinHostPort(cfg.Static.Storage.Host, strconv.Itoa(cfg.Static.Storage.Port))
	admin, err := sql.Open("mysql", d.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "gateway_oauth_test_" + security.Random()[:16]
	if _, err = admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if !strings.HasPrefix(name, "gateway_oauth_test_") {
			panic("unsafe scratch database")
		}
		admin.Exec("DROP DATABASE `" + name + "`")
	}()
	cfg.Static.Storage.Database = name
	cfg.Static.Auth.Gateway.Registration.Clients = nil
	store, err := persistence.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.SQL.Close()
	ctx := context.Background()
	provider, err := goose.NewProvider(goose.DialectMySQL, store.SQL, os.DirFS("migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SQL.Exec("INSERT INTO wiki_quota_locks(id) VALUES (?)", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SQL.Exec("INSERT INTO oauth_accounts(id,expires_at,updated_at,payload) VALUES (?,UTC_TIMESTAMP(),UTC_TIMESTAMP(),?)", strings.Repeat("b", 64), []byte("old-encrypted-payload")); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = store.SQL.QueryRow("SELECT COUNT(*) FROM oauth_accounts").Scan(&n); err != nil || n != 0 {
		t.Fatal("old OAuth records retained", err, n)
	}
	if _, err = provider.DownTo(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.CheckSchema(); err != nil {
		t.Fatal(err)
	}
	if err = store.SQL.QueryRow("SELECT COUNT(*) FROM wiki_quota_locks").Scan(&n); err != nil || n != 1 {
		t.Fatal("Wiki data changed", err, n)
	}
	svc := &application.Service{Config: cfg, Store: mysqlrepo.New(store)}
	if err = svc.SeedClients(ctx); err != nil {
		t.Fatal(err)
	}
	client := func(port int) *domain.Client {
		return &domain.Client{Name: "OpenCode", URI: "https://opencode.ai", Redirects: []string{fmt.Sprintf("http://127.0.0.1:%d/callback", port)}, Grants: []string{"authorization_code", "refresh_token"}}
	}
	c := client(1234)
	if err = svc.Register(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c.LogoURL == nil || *c.LogoURL != "" || c.TrustLevel != domain.TrustUnknown {
		t.Fatal("DCR defaults")
	}
	var first persistence.Row
	store.DB.Table("oauth_clients").Where("id = ?", security.Hash(c.ID)).Take(&first)
	if first.Fingerprint != c.Fingerprint {
		t.Fatal("fingerprint column not persisted")
	}
	cfg.Static.Auth.Gateway.Registration.MaxClients = 1
	errors := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			other := client(port)
			if err := svc.Register(ctx, other); err != nil {
				errors <- err
			} else if other.ID != c.ID {
				errors <- fmt.Errorf("different client ID")
			}
		}(3000 + i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var again persistence.Row
	store.DB.Table("oauth_clients").Where("id = ?", first.ID).Take(&again)
	if !again.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatal("pending expiry extended")
	}
	logo := "https://images.example/first.png"
	other := client(6000)
	other.LogoURL = &logo
	if err = svc.Register(ctx, other); err != nil {
		t.Fatal(err)
	}
	if other.LogoURL == nil || *other.LogoURL != "" {
		t.Fatal("locked empty logo overwritten")
	}
	if err = store.DB.Table("oauth_clients").Where("id = ?", first.ID).Updates(map[string]any{"logo_url": nil, "trust_level": "known"}).Error; err != nil {
		t.Fatal(err)
	}
	other = client(6001)
	if err = svc.Register(ctx, other); err != nil {
		t.Fatal(err)
	}
	if other.LogoURL != nil || other.TrustLevel != domain.TrustKnown {
		t.Fatal("NULL or admin trust lost")
	}
	errors = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := client(7000 + i)
			url := fmt.Sprintf("https://images.example/%d.png", i)
			next.LogoURL = &url
			if err := svc.Register(ctx, next); err != nil {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	loaded, err := svc.GetClientByClientID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LogoURL == nil || *loaded.LogoURL == "" {
		t.Fatal("NULL logo not filled")
	}
	locked := *loaded.LogoURL
	other = client(8000)
	other.LogoURL = &logo
	if err = svc.Register(ctx, other); err != nil {
		t.Fatal(err)
	}
	if *other.LogoURL != locked {
		t.Fatal("logo overwritten")
	}
	cfg.Static.Auth.Gateway.Registration.MaxClients = 100
	if err = store.DB.Table("oauth_clients").Where("id = ?", first.ID).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	fresh := client(9000)
	if err = svc.Register(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.ID == c.ID {
		t.Fatal("expired pending reused")
	}
	// Concurrent first-time registration must create exactly one row.
	ids := make(chan string, 8)
	errors = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := client(10000 + i)
			v.Name = "Concurrent first registration"
			if err := svc.Register(ctx, v); err != nil {
				errors <- err
			} else {
				ids <- v.ID
			}
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	one := ""
	for id := range ids {
		if one == "" {
			one = id
		}
		if id != one {
			t.Fatal("concurrent first registration duplicated")
		}
	}
	// A valid active record takes precedence over an equivalent pending record.
	active := *fresh
	active.ID = "active-equivalent"
	active.TrustLevel = domain.TrustUnknown
	if err = svc.PersistClient(svc.Store.Database(), &active, "active", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	chosen := client(11000)
	if err = svc.Register(ctx, chosen); err != nil {
		t.Fatal(err)
	}
	if chosen.ID != active.ID {
		t.Fatal("active not preferred")
	}
	if err = store.DB.Table("oauth_clients").Where("id = ?", security.Hash(active.ID)).Update("state", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	t.Run("blocked_registration", func(t *testing.T) {
		makeClient := func() *domain.Client {
			return &domain.Client{Name: "WorkBuddy Connector", Redirects: []string{"workbuddy://workbuddy/mcp/custom-mcp%3Aisekaiwiki/oauth/callback"}, Grants: []string{"authorization_code", "refresh_token"}, Responses: []string{"code"}, TokenAuthMethod: "none", Scope: "mcp:access"}
		}
		blocked := makeClient()
		if err := svc.Register(ctx, blocked); err != nil {
			t.Fatal(err)
		}
		if blocked.TrustLevel != domain.TrustBlocked {
			t.Fatal("not blocked")
		}
		var before persistence.Row
		if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(blocked.ID)).Take(&before).Error; err != nil {
			t.Fatal(err)
		}
		if before.State != "pending" || time.Until(before.ExpiresAt) > 24*time.Hour {
			t.Fatal("pending expiry changed")
		}
		if _, err := svc.GetClientByClientID(ctx, blocked.ID); err == nil {
			t.Fatal("blocked client accepted")
		}
		if err := store.DB.Table("oauth_clients").Where("id = ?", before.ID).Update("trust_level", "unknown").Error; err != nil {
			t.Fatal(err)
		}
		retry := makeClient()
		if err := svc.Register(ctx, retry); err != nil {
			t.Fatal(err)
		}
		if retry.ID != blocked.ID || retry.TrustLevel != domain.TrustUnknown {
			t.Fatal("admin approval overwritten")
		}
		var after persistence.Row
		store.DB.Table("oauth_clients").Where("id = ?", before.ID).Take(&after)
		if !after.ExpiresAt.Equal(before.ExpiresAt) {
			t.Fatal("retry extended expiry")
		}
		if _, err := svc.GetClientByClientID(ctx, retry.ID); err != nil {
			t.Fatal(err)
		}
		other := makeClient()
		other.Name = "Other app"
		if err := svc.Register(ctx, other); err != nil {
			t.Fatal(err)
		}
		if other.TrustLevel != domain.TrustBlocked {
			t.Fatal("scheme globally approved")
		}
		if err := store.DB.Table("oauth_clients").Where("id = ?", before.ID).Update("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
		fresh := makeClient()
		if err := svc.Register(ctx, fresh); err != nil {
			t.Fatal(err)
		}
		if fresh.ID == retry.ID || fresh.TrustLevel != domain.TrustBlocked {
			t.Fatal("expired approval reused")
		}
	})
	t.Run("authorization_and_tokens", func(t *testing.T) {
		workbuddy := &domain.Client{Name: "WorkBuddy flow", Redirects: []string{"workbuddy://workbuddy/mcp/custom-mcp%3Aisekaiwiki/oauth/callback"}, Grants: []string{"authorization_code", "refresh_token"}, Scope: "mcp:access offline_access"}
		if err := svc.Register(ctx, workbuddy); err != nil {
			t.Fatal(err)
		}
		if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(workbuddy.ID)).Update("trust_level", "unknown").Error; err != nil {
			t.Fatal(err)
		}
		allowed, err := svc.GetClientByClientID(ctx, workbuddy.ID)
		if err != nil {
			t.Fatal(err)
		}
		testAuthorization(t, svc, store, allowed)
	})
	t.Run("blocked_migration_roundtrip", func(t *testing.T) {
		id := security.Hash(fresh.ID)
		if err := store.DB.Table("oauth_clients").Where("id = ?", id).Update("trust_level", "blocked").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := provider.DownTo(ctx, 3); err != nil {
			t.Fatal(err)
		}
		var level string
		if err := store.SQL.QueryRow("SELECT trust_level FROM oauth_clients WHERE id = ?", id).Scan(&level); err != nil || level != "unknown" {
			t.Fatal("rollback failed", level, err)
		}
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.GetClientByClientID(ctx, fresh.ID); err != nil {
			t.Fatal("migration lost client", err)
		}
	})
	t.Run("client_column_authority", func(t *testing.T) {
		loaded, err := svc.GetClientByClientID(ctx, fresh.ID)
		if err != nil {
			t.Fatal(err)
		}
		store.DB.Table("oauth_clients").Where("id = ?", security.Hash(loaded.ID)).Updates(map[string]any{"trust_level": "trusted", "logo_url": nil})
		loaded, _, err = svc.LoadClient(svc.Store.Database(), loaded.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if err = svc.PersistClient(svc.Store.Database(), loaded, "active", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		after, _, err := svc.LoadClient(svc.Store.Database(), loaded.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if after.LogoURL != nil || after.TrustLevel != domain.TrustTrusted {
			t.Fatal("admin metadata overwritten")
		}
	})
}

func testAuthorization(t *testing.T, s *application.Service, store *persistence.Store, c *domain.Client) {
	t.Helper()
	ctx := context.Background()
	db := s.Store.Database()
	exp := time.Now().UTC().Add(time.Hour)
	account := domain.Account{ID: "account", Username: "Alice", WikiUserID: 9007199254740993}
	session := domain.Session{ID: "private-session", AccountID: account.ID, Browser: "browser", Identity: domain.Identity{UserToken: "private-user-token"}}
	if err := s.Store.Create(db, "accounts", account.ID, account, persistence.Row{AccountID: account.ID, State: "active", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.Create(db, "sessions", session.ID, session, persistence.Row{AccountID: account.ID, State: "active", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	t.Run("administrator_trusted_pending", func(t *testing.T) {
		v := *c
		v.ID = ""
		v.Name = "Administrator trusted WorkBuddy"
		if err := s.Register(ctx, &v); err != nil {
			t.Fatal(err)
		}
		if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(v.ID)).Update("trust_level", "trusted").Error; err != nil {
			t.Fatal(err)
		}
		trusted, err := s.GetClientByClientID(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		reqctx := context.WithValue(ctx, application.RequestKey{}, application.RequestInfo{Browser: "browser", Resource: "resource", ClientID: v.ID})
		interaction, err := s.CreateAuthRequest(reqctx, &domain.AuthorizationRequest{ClientID: v.ID, RedirectURI: v.Redirects[0], Scopes: domain.SpaceDelimited{"mcp:access"}}, "")
		if err != nil {
			t.Fatal(err)
		}
		interaction.AccountID = account.ID
		interaction.SessionID = session.ID
		if err := s.Store.Put(db, "interactions", interaction.ID, interaction, persistence.Row{AccountID: account.ID, State: "authenticated", ExpiresAt: exp}); err != nil {
			t.Fatal(err)
		}
		if err := s.Consent(reqctx, interaction, trusted, []string{"mcp:access"}); err != nil {
			t.Fatal(err)
		}
		_, row, err := s.LoadClient(db, v.ID, false)
		if err != nil || row.State != "active" {
			t.Fatal("trusted pending not activated", err)
		}
		// Keep this separate flow out of the management-list assertions below.
		loaded, err := s.AuthRequestByID(reqctx, interaction.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Store.Delete(db, "grants", loaded.GrantID); err != nil {
			t.Fatal(err)
		}
	})
	ctx = context.WithValue(ctx, application.RequestKey{}, application.RequestInfo{Browser: "browser", Resource: "resource", ClientID: c.ID})
	a, err := s.CreateAuthRequest(ctx, &domain.AuthorizationRequest{ClientID: c.ID, RedirectURI: c.Redirects[0], Scopes: domain.SpaceDelimited{"mcp:access"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	a.AccountID = account.ID
	a.SessionID = session.ID
	a.TrustAfter = time.Now().Add(-time.Second)
	if err = s.Store.Put(db, "interactions", a.ID, a, persistence.Row{AccountID: account.ID, State: "authenticated", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	setLevel := func(level string) {
		t.Helper()
		if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(c.ID)).Update("trust_level", level).Error; err != nil {
			t.Fatal(err)
		}
	}
	setLevel("blocked")
	if err := s.ConfirmTrust(ctx, a, c); err == nil {
		t.Fatal("blocked authenticated interaction confirmed trust")
	}
	setLevel("unknown")
	if err = s.ConfirmTrust(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	if err = s.Consent(ctx, a, c, []string{"mcp:access"}); err != nil {
		t.Fatal(err)
	}
	a, err = s.AuthRequestByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAuthCode(ctx, a.ID, "private-code"); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, application.RequestKey{}, application.RequestInfo{Resource: "resource", ClientID: c.ID, Code: "private-code"})
	setLevel("blocked")
	if _, _, _, err := s.CreateAccessAndRefreshTokens(ctx, a, ""); err == nil {
		t.Fatal("blocked unconsumed code exchanged")
	}
	setLevel("unknown")
	access, refresh, _, err := s.CreateAccessAndRefreshTokens(ctx, a, "")
	if err != nil {
		t.Fatal(err)
	}
	var row persistence.Row
	if err = store.DB.Table("oauth_access_tokens").Where("id = ?", security.Hash(access)).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(row.Payload), access) || len(row.EncryptedPayload) == 0 {
		t.Fatal("raw token leaked")
	}
	token, err := s.TokenRequestByRefreshToken(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	_, next, _, err := s.CreateAccessAndRefreshTokens(ctx, token, refresh)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"grants", "client_trusts"} {
		entries, err := s.ListManaged(ctx, account.ID, kind)
		if err != nil || len(entries) != 1 {
			t.Fatal("management list", kind, err)
		}
		if entries[0].TrustLevel != domain.TrustUnknown {
			t.Fatal("trust confirmation changed presentation level")
		}
	}
	if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(c.ID)).Update("trust_level", "blocked").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.AccessByID(db, access, account.ID); err == nil {
		t.Fatal("blocked access accepted")
	}
	if _, _, err := s.ActiveGrant(db, a.GrantID, false); err == nil {
		t.Fatal("blocked grant accepted")
	}
	if _, err := s.TokenRequestByRefreshToken(ctx, next); err == nil {
		t.Fatal("blocked refresh accepted")
	}
	if err := s.ConfirmTrust(ctx, a, c); err == nil {
		t.Fatal("blocked trust accepted")
	}
	if err := s.Consent(ctx, a, c, []string{"mcp:access"}); err == nil {
		t.Fatal("blocked consent accepted")
	}
	if err := s.SaveAuthCode(ctx, a.ID, "blocked-code"); err == nil {
		t.Fatal("blocked code issued")
	}
	if _, err := s.AuthRequestByCode(ctx, "private-code"); err == nil {
		t.Fatal("blocked code accepted")
	}
	if err := store.DB.Table("oauth_clients").Where("id = ?", security.Hash(c.ID)).Update("trust_level", "unknown").Error; err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeToken(ctx, next, account.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TokenRequestByRefreshToken(ctx, next); err == nil {
		t.Fatal("revoked refresh accepted")
	}
	// Login snapshots must retain both their opaque bytes and PKCE secret.
	login := domain.LoginTransaction{ID: "state", Verifier: "pkce-private", Snapshot: []byte(`{"secret":"upstream-private"}`)}
	if err = s.Store.Create(db, "login_transactions", login.ID, login, persistence.Row{State: "active", ExpiresAt: exp}); err != nil {
		t.Fatal(err)
	}
	var recovered domain.LoginTransaction
	if _, err = s.Store.Get(db, "login_transactions", login.ID, &recovered, false); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(login)
	other, _ := json.Marshal(recovered)
	if string(b) != string(other) {
		t.Fatal("login snapshot changed")
	}
}
