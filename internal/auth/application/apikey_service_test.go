package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

type domainAPIKey = domain.APIKey

// apikeyRow 是测试内存中的 api_keys 行。
type apikeyRow struct {
	Row              persistence.Row
	Payload          []byte
	EncryptedPayload []byte
}

// apikeyDB 支持本服务用到的查询模式。
type apikeyDB struct {
	store *apikeyStore
	table string
	pred  string
	cond  []any
	err   error
}

func (d *apikeyDB) WithContext(context.Context) Database      { return d }
func (d *apikeyDB) Transaction(fn func(Database) error) error { return fn(d) }
func (d *apikeyDB) Table(t string) Database {
	d.table, d.pred, d.cond, d.err = t, "", nil, nil
	return d
}
func (d *apikeyDB) Where(pred any, args ...any) Database {
	d.pred, d.cond, d.err = pred.(string), args, nil
	return d
}
func (d *apikeyDB) Order(any) Database { return d }
func (d *apikeyDB) Lock() Database     { return d }
func (d *apikeyDB) Update(_ string, val any) Database {
	for i := range d.store.rows {
		if d.table == persistence.Table("api_keys") && d.matches(d.store.rows[i]) {
			d.store.rows[i].Row.State = val.(string)
		}
	}
	return d
}
func (d *apikeyDB) Delete(any) Database {
	kept := d.store.rows[:0]
	for _, row := range d.store.rows {
		if !d.matches(row) {
			kept = append(kept, row)
		}
	}
	d.store.rows = kept
	return d
}
func (d *apikeyDB) Take(value any) Database {
	for _, row := range d.store.rows {
		if d.matches(row) {
			switch v := value.(type) {
			case *persistence.Row:
				*v = row.Row
			default:
				_ = json.Unmarshal(row.Payload, value)
			}
			return d
		}
	}
	d.err = ErrRecordNotFound
	return d
}
func (d *apikeyDB) Find(value any) Database {
	var out []persistence.Row
	for _, row := range d.store.rows {
		if d.matches(row) {
			out = append(out, row.Row)
		}
	}
	if rows, ok := value.(*[]persistence.Row); ok {
		*rows = out
	}
	return d
}
func (d *apikeyDB) Count(n *int64) Database {
	var total int64
	for _, row := range d.store.rows {
		if d.matches(row) {
			total++
		}
	}
	*n = total
	return d
}
func (d *apikeyDB) Error() error { return d.err }

// matches 按本服务使用的谓词判定行匹配。
func (d *apikeyDB) matches(row apikeyRow) bool {
	if d.table != "" && d.table != persistence.Table("api_keys") {
		return false
	}
	switch {
	case d.pred == "account_id = ? AND state = ? AND expires_at > ?":
		return len(d.cond) == 3 && row.Row.AccountID == d.cond[0] && row.Row.State == d.cond[1] && row.Row.ExpiresAt.After(d.cond[2].(time.Time))
	case d.pred == "account_id = ? AND state = ?":
		return len(d.cond) == 2 && row.Row.AccountID == d.cond[0] && row.Row.State == d.cond[1]
	case d.pred == "id = ? AND account_id = ?":
		return len(d.cond) == 2 && row.Row.ID == d.cond[0] && row.Row.AccountID == d.cond[1]
	case d.pred == "id = ? AND state = ?":
		return len(d.cond) == 2 && row.Row.ID == d.cond[0] && row.Row.State == d.cond[1]
	case d.pred == "id = ?":
		return len(d.cond) == 1 && row.Row.ID == d.cond[0]
	}
	return d.pred == ""
}

// apikeyStore 内存 RecordStore：Identity.user_token 与真实仓库同样分离加密。
type apikeyStore struct {
	RecordStore
	rows     []apikeyRow
	accounts map[string]Account
}

func (s *apikeyStore) Database() Database { return &apikeyDB{store: s} }

func (s *apikeyStore) getAccount(id string) (Account, bool) {
	if s.accounts == nil {
		return Account{}, false
	}
	a, ok := s.accounts[id]
	return a, ok
}

// seal 模拟 Vault 的凭据拆分：Identity.user_token 移入独立密文。
func sealPayload(value any) ([]byte, []byte) {
	var plain map[string]any
	b, _ := json.Marshal(value)
	_ = json.Unmarshal(b, &plain)
	identity, _ := plain["Identity"].(map[string]any)
	secret := map[string]any{}
	if identity != nil {
		if token, ok := identity["user_token"]; ok && token != "" {
			secret["user_token"] = token
			delete(identity, "user_token")
		}
	}
	plain["Identity"] = identity
	b, _ = json.Marshal(plain)
	encrypted, _ := json.Marshal(secret)
	return b, encrypted
}

func (s *apikeyStore) Get(_ Database, kind, key string, value any, _ bool) (persistence.Row, error) {
	id := security.Hash(key)
	for _, row := range s.rows {
		if row.Row.ID != id {
			continue
		}
		merged := row.Payload
		if len(row.EncryptedPayload) > 0 {
			var plain, secret map[string]any
			_ = json.Unmarshal(row.Payload, &plain)
			_ = json.Unmarshal(row.EncryptedPayload, &secret)
			identity, _ := plain["Identity"].(map[string]any)
			if identity != nil {
				if token, ok := secret["user_token"]; ok {
					identity["user_token"] = token
				}
			}
			merged, _ = json.Marshal(plain)
		}
		switch {
		case kind == "api_keys":
			_ = json.Unmarshal(merged, value.(*domainAPIKey))
		default:
			_ = json.Unmarshal(merged, value)
		}
		return row.Row, nil
	}
	if kind == "accounts" {
		if a, ok := s.getAccount(key); ok {
			*value.(*Account) = a
			return persistence.Row{ID: security.Hash(key), State: "active"}, nil
		}
	}
	return persistence.Row{}, ErrRecordNotFound
}

func (s *apikeyStore) Create(_ Database, kind, key string, value any, meta persistence.Row) error {
	payload, encrypted := sealPayload(value)
	meta.ID, meta.UpdatedAt, meta.Payload, meta.EncryptedPayload = security.Hash(key), time.Now().UTC(), payload, encrypted
	s.rows = append(s.rows, apikeyRow{Row: meta, Payload: payload, EncryptedPayload: encrypted})
	return nil
}

func (s *apikeyStore) Put(_ Database, kind, key string, value any, meta persistence.Row) error {
	payload, encrypted := sealPayload(value)
	meta.ID, meta.UpdatedAt, meta.Payload, meta.EncryptedPayload = security.Hash(key), time.Now().UTC(), payload, encrypted
	for i := range s.rows {
		if s.rows[i].Row.ID == meta.ID {
			s.rows[i] = apikeyRow{Row: meta, Payload: payload, EncryptedPayload: encrypted}
			return nil
		}
	}
	s.rows = append(s.rows, apikeyRow{Row: meta, Payload: payload, EncryptedPayload: encrypted})
	return nil
}

func (s *apikeyStore) OpenRecord(row persistence.Row, value any, kind string) error {
	if kind != "api_keys" {
		return nil
	}
	for _, r := range s.rows {
		if r.Row.ID != row.ID {
			continue
		}
		merged := r.Payload
		if len(r.EncryptedPayload) > 0 {
			var plain, secret map[string]any
			_ = json.Unmarshal(r.Payload, &plain)
			_ = json.Unmarshal(r.EncryptedPayload, &secret)
			identity, _ := plain["Identity"].(map[string]any)
			if identity != nil {
				if token, ok := secret["user_token"]; ok {
					identity["user_token"] = token
				}
			}
			merged, _ = json.Marshal(plain)
		}
		return json.Unmarshal(merged, value)
	}
	return ErrRecordNotFound
}

func newKeyService(t *testing.T, apiKeys *config.APIKeys) *Service {
	t.Helper()
	c := config.Defaults()
	if apiKeys != nil {
		c.Static.Auth.Gateway.APIKeys = *apiKeys
	}
	return &Service{Config: c, Store: &apikeyStore{accounts: map[string]Account{"account": {ID: "account", Subject: "subject"}}}}
}

func TestCreateAndValidateAPIKey(t *testing.T) {
	s := newKeyService(t, nil)
	session := &Session{ID: "session", AccountID: "account", Identity: domain.Identity{UserID: 7, Username: "alice", UserToken: "wiki-token"}}
	plaintext, key, err := s.CreateAPIKey(context.Background(), session, " desktop ", []string{"wiki:edit"}, permanentExpiry())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plaintext, "sk-") || len(plaintext) != 3+apiKeyIDLen+43 {
		t.Fatalf("plaintext format: %q", plaintext)
	}
	// scope 归一化后必须恒含 mcp:access 与隐含的 wiki:read，且未授权范围被剔除。
	if !containsScope(key.Scopes, "mcp:access") || !containsScope(key.Scopes, "wiki:read") || containsScope(key.Scopes, "wiki:upload") {
		t.Fatalf("unexpected scopes: %v", key.Scopes)
	}
	// 落库只有哈希：明文 sk secret 不得出现在 payload；会话 UserToken 走独立加密。
	secret := plaintext[3+apiKeyIDLen:]
	digest := sha256.Sum256([]byte(secret))
	if key.SecretHash != hex.EncodeToString(digest[:]) {
		t.Fatal("secret hash mismatch")
	}
	if err := s.Store.(*apikeyStore).assertCredentialsSealed(t, plaintext[3:3+apiKeyIDLen], secret, session.Identity.UserToken); err != nil {
		t.Fatal(err)
	}
	p, err := s.ValidateAPIKey(context.Background(), plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if p.AuthKind != domain.AuthKindAPIKey || p.AccountID != "account" || p.OAuthClientID != "api-key" || p.GrantID != plaintext[3:3+apiKeyIDLen] || p.WikiUserID != 7 {
		t.Fatalf("principal: %+v", p)
	}
	// 篡改 secret 段必须失败。
	if _, err := s.ValidateAPIKey(context.Background(), plaintext[:3+apiKeyIDLen]+strings.Repeat("x", 43)); err == nil {
		t.Fatal("forged secret accepted")
	}
	// 未知的 id 段必须失败。
	if _, err := s.ValidateAPIKey(context.Background(), "sk-"+strings.Repeat("0", apiKeyIDLen)+secret); err == nil {
		t.Fatal("unknown key accepted")
	}
	// 禁用后 sk 分支一律视为 invalid token。
	s.Config.Static.Auth.Gateway.APIKeys.Enabled = false
	if _, err := s.ValidateAPIKey(context.Background(), plaintext); err == nil {
		t.Fatal("disabled gateway still accepted key")
	}
}

// assertCredentialsSealed 校验 sk secret 只存哈希，UserToken 只存密文。
func (s *apikeyStore) assertCredentialsSealed(t *testing.T, keyID, secret, userToken string) error {
	t.Helper()
	for _, row := range s.rows {
		if row.Row.ID != security.Hash(keyID) {
			continue
		}
		if strings.Contains(string(row.Payload), secret) {
			return errors.New("plaintext secret stored in payload")
		}
		if strings.Contains(string(row.Payload), userToken) {
			return errors.New("user token stored in plaintext payload")
		}
		if !strings.Contains(string(row.EncryptedPayload), userToken) {
			return errors.New("user token not split into encrypted payload")
		}
		return nil
	}
	return errors.New("key row missing")
}

func TestAPIKeyExpiryAndRevoke(t *testing.T) {
	s := newKeyService(t, &config.APIKeys{Enabled: true, MaxPerUser: 10, MaxTTLSeconds: 31536000, AllowPermanent: false})
	session := &Session{ID: "session", AccountID: "account", Identity: domain.Identity{UserID: 7, UserToken: "t"}}
	if _, _, err := s.CreateAPIKey(context.Background(), session, "k", nil, permanentExpiry()); err == nil {
		t.Fatal("permanent key allowed while disabled")
	}
	plaintext, _, err := s.CreateAPIKey(context.Background(), session, "k", nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// 回写行过期时间模拟已过期密钥。
	s.Store.(*apikeyStore).rows[0].Row.ExpiresAt = time.Now().Add(-time.Minute)
	if _, err := s.ValidateAPIKey(context.Background(), plaintext); err == nil {
		t.Fatal("expired key accepted")
	}
	// TTL 超限必须拒绝。
	if _, _, err := s.CreateAPIKey(context.Background(), session, "k", nil, time.Now().Add(time.Duration(31536000+86400)*time.Second)); err == nil {
		t.Fatal("over-limit TTL accepted")
	}
	// 吊销后校验失败，且不允许删除他人 key。
	plaintext2, _, err := s.CreateAPIKey(context.Background(), session, "k2", nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	rows := s.Store.(*apikeyStore).rows
	// 不允许删除他人账号的 key。
	if err := s.DeleteAPIKey(context.Background(), "other", rows[1].Row.ID); err == nil {
		t.Fatal("foreign account deleted a key")
	}
	if s.Store.(*apikeyStore).rows[1].Row.State != "active" {
		t.Fatal("foreign account revoked a key")
	}
	if err := s.DeleteAPIKey(context.Background(), "account", rows[1].Row.ID); err != nil {
		t.Fatal(err)
	}
	if s.Store.(*apikeyStore).rows[1].Row.State != "revoked" {
		t.Fatal("key not revoked")
	}
	keys, _, err := s.ListAPIKeys(context.Background(), "account")
	if err != nil || len(keys) != 1 || keys[0].Name != "k" {
		t.Fatalf("revoked key remained in list: %#v, %v", keys, err)
	}
	if _, err := s.ValidateAPIKey(context.Background(), plaintext2); err == nil {
		t.Fatal("revoked key accepted")
	}
	if _, err := s.ValidateAPIKey(context.Background(), plaintext); err == nil {
		t.Fatal("expired key accepted again")
	}
}

func TestAPIKeyPerUserLimit(t *testing.T) {
	s := newKeyService(t, &config.APIKeys{Enabled: true, MaxPerUser: 1, MaxTTLSeconds: 31536000, AllowPermanent: true})
	session := &Session{ID: "session", AccountID: "account", Identity: domain.Identity{UserID: 7, UserToken: "t"}}
	if _, _, err := s.CreateAPIKey(context.Background(), session, "one", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateAPIKey(context.Background(), session, "two", nil, time.Now().Add(time.Hour)); err == nil {
		t.Fatal("limit not enforced")
	}
}

func TestRefreshAPIKeyIdentity(t *testing.T) {
	var subject string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject = r.Header.Get("X-Isekai-Subject")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gatewayservice":{"userinfo":{"user_id":9,"username":"bob","user_token":"new-token"}}}`))
	}))
	defer srv.Close()
	s := newKeyService(t, nil)
	c := config.Defaults()
	c.App.MediaWiki.APIURL = srv.URL
	s.Upstream = new(outbound.Manager)
	s.Upstream.Publish(&outbound.Bundle{Config: c, Bridge: &mediawiki.Client{Config: c.App.MediaWiki, Secret: c.Secrets.BridgeSecret, HTTP: srv.Client()}})
	session := &Session{ID: "session", AccountID: "account", Identity: domain.Identity{UserID: 7, Username: "alice", UserToken: "old"}}
	_, key, err := s.CreateAPIKey(context.Background(), session, "k", nil, permanentExpiry())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshAPIKeyIdentity(context.Background(), key.ID); err != nil {
		t.Fatal(err)
	}
	if subject != "subject" {
		t.Fatalf("resolve used subject %q", subject)
	}
	// 刷新后的快照写回，且新 UserToken 走独立加密分离。
	store := s.Store.(*apikeyStore)
	var stored domainAPIKey
	if err := json.Unmarshal(store.rows[0].Payload, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Identity.UserID != 9 || stored.Identity.Username != "bob" || stored.Identity.UserToken != "" {
		t.Fatalf("identity snapshot not refreshed or token leaked: %+v", stored.Identity)
	}
	if !strings.Contains(string(store.rows[0].EncryptedPayload), "new-token") {
		t.Fatal("refreshed token not split into encrypted payload")
	}
}
