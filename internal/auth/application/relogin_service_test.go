package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

// reloginStore 是覆盖 AttachSession/ReuseGrant 状态机的内存 RecordStore。
type reloginStore struct {
	RecordStore
	interactions map[string]struct {
		value Interaction
		row   persistence.Row
	}
	grants    []struct {
		key     string
		payload []byte
		row     persistence.Row
	}
	sessions map[string]persistence.Row
	clients  map[string]struct {
		value storedClient
		row   persistence.Row
	}
	accounts map[string]persistence.Row
}

func newReloginStore() *reloginStore {
	return &reloginStore{
		interactions: map[string]struct {
			value Interaction
			row   persistence.Row
		}{},
		sessions:  map[string]persistence.Row{},
		clients:   map[string]struct {
			value storedClient
			row   persistence.Row
		}{},
		accounts: map[string]persistence.Row{},
	}
}

func (s *reloginStore) Get(_ Database, kind, key string, value any, _ bool) (persistence.Row, error) {
	switch kind {
	case "interactions":
		if v, ok := s.interactions[key]; ok {
			*value.(*Interaction) = v.value
			return v.row, nil
		}
	case "grants":
		for _, g := range s.grants {
			if g.key == security.Hash(key) {
				return g.row, json.Unmarshal(g.payload, value)
			}
		}
	case "sessions":
		if r, ok := s.sessions[key]; ok {
			return r, nil
		}
	case "clients":
		if v, ok := s.clients[key]; ok {
			*value.(*storedClient) = v.value
			return v.row, nil
		}
	case "accounts":
		if r, ok := s.accounts[key]; ok {
			return r, nil
		}
	case "client_trusts":
		// 测试客户端均为 trusted，不落信任记录。
	}
	return persistence.Row{}, ErrRecordNotFound
}

func (s *reloginStore) Create(_ Database, kind, key string, value any, meta persistence.Row) error {
	payload, _ := json.Marshal(value)
	meta.ID, meta.UpdatedAt, meta.Payload = security.Hash(key), time.Now().UTC(), payload
	if kind == "grants" {
		s.grants = append(s.grants, struct {
			key     string
			payload []byte
			row     persistence.Row
		}{security.Hash(key), payload, meta})
	}
	return nil
}

func (s *reloginStore) Put(_ Database, kind, key string, value any, meta persistence.Row) error {
	payload, _ := json.Marshal(value)
	meta.ID, meta.UpdatedAt, meta.Payload = security.Hash(key), time.Now().UTC(), payload
	if kind == "interactions" {
		s.interactions[key] = struct {
			value Interaction
			row   persistence.Row
		}{*(value.(*Interaction)), meta}
	}
	return nil
}

func (s *reloginStore) OpenRecord(row persistence.Row, value any, kind string) error {
	if kind == "grants" {
		for _, g := range s.grants {
			if g.row.ID == row.ID {
				return json.Unmarshal(g.payload, value)
			}
		}
	}
	return nil
}

// reloginDB 支持 grants 的按 (account_id, client_id, state) 查询与 sessions 行取。
type reloginDB struct {
	store *reloginStore
	table string
	pred  string
	cond  []any
}

func (s *reloginStore) Database() Database { return &reloginDB{store: s} }

func (d *reloginDB) WithContext(context.Context) Database      { return d }
func (d *reloginDB) Transaction(fn func(Database) error) error { return fn(d) }
func (d *reloginDB) Table(t string) Database                   { *d = reloginDB{store: d.store, table: t}; return d }
func (d *reloginDB) Where(pred any, args ...any) Database      { d.pred, d.cond = pred.(string), args; return d }
func (d *reloginDB) Order(any) Database                        { return d }
func (d *reloginDB) Lock() Database                            { return d }
func (d *reloginDB) Update(_ string, val any) Database {
	if d.table == persistence.Table("grants") && d.pred == "id = ?" {
		for i := range d.store.grants {
			if d.store.grants[i].row.ID == d.cond[0] {
				d.store.grants[i].row.State = val.(string)
			}
		}
	}
	return d
}
func (d *reloginDB) Delete(any) Database                       { return d }
func (d *reloginDB) Count(*int64) Database                     { return d }
func (d *reloginDB) Error() error                              { return nil }

func (d *reloginDB) Take(value any) Database {
	if d.table == persistence.Table("sessions") && d.pred == "id = ?" {
		if r, ok := d.store.sessions[d.cond[0].(string)]; ok {
			*value.(*persistence.Row) = r
			return d
		}
	}
	return d
}

func (d *reloginDB) Find(value any) Database {
	if d.table != persistence.Table("grants") || d.pred != "account_id = ? AND client_id = ? AND state = ?" {
		return d
	}
	var out []persistence.Row
	for _, g := range d.store.grants {
		if g.row.AccountID == d.cond[0] && g.row.ClientID == d.cond[1] && g.row.State == d.cond[2] {
			out = append(out, g.row)
		}
	}
	if rows, ok := value.(*[]persistence.Row); ok {
		*rows = out
	}
	return d
}

func newReloginService(t *testing.T, store *reloginStore) *Service {
	t.Helper()
	c := config.Defaults()
	return &Service{Config: c, Store: store}
}

func reloginFixtures(t *testing.T, store *reloginStore) (interactionID, resource string) {
	t.Helper()
	now := time.Now().UTC()
	// trusted 预注册客户端。
	client := Client{ID: "client", Name: "App", Scope: "mcp:access wiki:read wiki:edit", Fingerprint: "fingerprint", Source: "pre", TrustLevel: domain.TrustTrusted}
	store.clients[client.ID] = struct {
		value storedClient
		row   persistence.Row
	}{storedClient{Metadata: client, Source: "pre", Fingerprint: client.Fingerprint}, persistence.Row{State: "active", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}}
	store.accounts["account"] = persistence.Row{State: "active", ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}
	store.sessions[security.Hash("session")] = persistence.Row{State: "active", ExpiresAt: now.Add(time.Hour)}
	store.sessions[security.Hash("old-session")] = persistence.Row{State: "revoked", ExpiresAt: now.Add(-time.Hour)}
	resource = "https://gateway.example/mcp"
	a := Interaction{ID: "interaction", Resource: resource, Browser: "browser", ClientFingerprint: "fingerprint"}
	a.Request = domain.AuthorizationRequest{ClientID: "client", Scopes: []string{"mcp:access", "wiki:read", "wiki:edit"}}
	store.interactions["interaction"] = struct {
		value Interaction
		row   persistence.Row
	}{a, persistence.Row{State: "login", ExpiresAt: now.Add(time.Hour)}}
	// 旧授权：绑定了已死会话，scopes 与请求一致。
	old := Grant{ID: "old-grant", AccountID: "account", ClientID: "client", Resource: resource, Scopes: []string{"mcp:access", "wiki:read", "wiki:edit", "offline_access"}, SessionID: "old-session", Fingerprint: "fingerprint", AuthTime: now}
	payload, _ := json.Marshal(old)
	store.grants = append(store.grants, struct {
		key     string
		payload []byte
		row     persistence.Row
	}{security.Hash("old-grant"), payload, persistence.Row{ID: security.Hash("old-grant"), AccountID: "account", ClientID: security.Hash("client"), GrantID: "old-grant", State: "active", ExpiresAt: now.Add(time.Hour), UpdatedAt: now}})
	return "interaction", resource
}

func TestAttachSessionAndReuseGrant(t *testing.T) {
	store := newReloginStore()
	id, resource := reloginFixtures(t, store)
	s := newReloginService(t, store)
	ctx := context.Background()

	// AttachSession：绑定活跃会话并推进状态机。
	session := &Session{ID: "session", AccountID: "account", Browser: "browser", Identity: domain.Identity{UserID: 5}}
	if err := s.AttachSession(ctx, id, session); err != nil {
		t.Fatal(err)
	}
	if got := store.interactions[id].value; got.AccountID != "account" || got.SessionID != "session" || got.AuthTime.IsZero() {
		t.Fatalf("attach did not bind session: %+v", got)
	}
	if store.interactions[id].row.State != "authenticated" {
		t.Fatal("interaction not authenticated")
	}

	// 重置回 login 状态后走一键沿用：ReuseGrant 自行绑定会话。
	fresh := store.interactions[id].value
	fresh.AccountID, fresh.SessionID, fresh.AuthTime = "", "", time.Time{}
	store.interactions[id] = struct {
		value Interaction
		row   persistence.Row
	}{fresh, persistence.Row{State: "login", ExpiresAt: store.interactions[id].row.ExpiresAt}}
	if err := s.ReuseGrant(ctx, id, session, &Client{ID: "client", Fingerprint: "fingerprint", Scope: "mcp:access", TrustLevel: domain.TrustTrusted}); err != nil {
		t.Fatal(err)
	}
	value, row := store.interactions[id].value, store.interactions[id].row
	if row.State != "consented" || !value.DoneFlag || value.GrantID == "" || value.GrantID == "old-grant" {
		t.Fatalf("reuse did not consent: %+v %+v", value, row)
	}
	// 旧授权已吊销，新授权活跃且绑定新会话。
	if store.grants[0].row.State != "revoked" {
		t.Fatal("old grant not revoked")
	}
	var issued Grant
	if _, err := s.Store.Get(store.Database(), "grants", value.GrantID, &issued, false); err != nil {
		t.Fatal(err)
	}
	if issued.SessionID != "session" || issued.Resource != resource || !strings.Contains(strings.Join(issued.Scopes, " "), "wiki:read") {
		t.Fatalf("new grant invalid: %+v", issued)
	}
}

func TestReuseGrantPreconditions(t *testing.T) {
	store := newReloginStore()
	id, _ := reloginFixtures(t, store)
	s := newReloginService(t, store)
	ctx := context.Background()
	session := &Session{ID: "session", AccountID: "account", Browser: "browser", Identity: domain.Identity{UserID: 5}}
	client := &Client{ID: "client", Fingerprint: "fingerprint", Scope: "mcp:access", TrustLevel: domain.TrustTrusted}

	// scopes 扩大：请求范围超出上次授权时不可沿用。
	a := store.interactions[id].value
	a.Request.Scopes = []string{"mcp:access", "wiki:read", "wiki:edit", "wiki:delete"}
	store.interactions[id] = struct {
		value Interaction
		row   persistence.Row
	}{a, store.interactions[id].row}
	if err := s.ReuseGrant(ctx, id, session, client); err == nil {
		t.Fatal("expanded scopes reused silently")
	}

	// 指纹不匹配的历史授权不可沿用。
	a.Request.Scopes = []string{"mcp:access", "wiki:read", "wiki:edit"}
	store.interactions[id] = struct {
		value Interaction
		row   persistence.Row
	}{a, store.interactions[id].row}
	if _, _, err := s.reusableGrant(store.Database(), "account", &Client{ID: "client", Fingerprint: "changed"}, a.Resource); err == nil {
		t.Fatal("stale fingerprint grant reused")
	}

	// 会话已死时不可沿用。
	store.sessions[security.Hash("session")] = persistence.Row{State: "revoked"}
	if err := s.ReuseGrant(ctx, id, session, client); err == nil {
		t.Fatal("dead session reuse accepted")
	}
}

