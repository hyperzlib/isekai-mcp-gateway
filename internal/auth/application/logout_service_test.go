package application

import (
	"context"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

type logoutDB struct {
	Database
	updates  []string
	table    string
	grantKey string
}

func (d *logoutDB) WithContext(context.Context) Database      { return d }
func (d *logoutDB) Transaction(fn func(Database) error) error { return fn(d) }
func (d *logoutDB) Table(t string) Database                   { d.table = t; return d }
func (d *logoutDB) Where(_ any, args ...any) Database {
	// 捕获 grant 表的查询键，用于断言吊销的是哈希后的授权 ID。
	if d.table == "oauth_grants" {
		d.grantKey = args[0].(string)
	}
	return d
}
func (d *logoutDB) Update(_ string, _ any) Database { d.updates = append(d.updates, d.table); return d }
func (d *logoutDB) Error() error                    { return nil }

type logoutStore struct {
	RecordStore
	db    *logoutDB
	grant Grant
}

func (s *logoutStore) Database() Database { return s.db }
func (s *logoutStore) Get(_ Database, kind, _ string, value any, _ bool) (persistence.Row, error) {
	switch kind {
	case "grants":
		*value.(*Grant) = s.grant
	case "clients":
		*value.(*storedClient) = storedClient{Metadata: Client{ID: "client"}, Source: "pre", Fingerprint: "fingerprint"}
	case "accounts":
		*value.(*Account) = Account{ID: "account"}
	case "sessions":
		*value.(*Session) = Session{AccountID: "account"}
	}
	return persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestLogoutBoundGrant(t *testing.T) {
	// 客户端不匹配时不得吊销授权；匹配时应级联吊销整个令牌族。
	for _, mismatch := range []bool{false, true} {
		db := new(logoutDB)
		store := &logoutStore{db: db, grant: Grant{ID: "grant", AccountID: "account", ClientID: "client", Fingerprint: "fingerprint", SessionID: "session"}}
		s := &Service{Store: store}
		p := &Principal{AccountID: "account", OAuthClientID: "client", GrantID: "grant"}
		if mismatch {
			p.OAuthClientID = "another-client"
		}
		err := s.Logout(context.Background(), p)
		if mismatch {
			if err == nil || len(db.updates) != 0 {
				t.Fatal("mismatched principal revoked a grant")
			}
		} else if err != nil || len(db.updates) != 4 || db.grantKey != security.Hash("grant") {
			t.Fatalf("grant and token family not revoked: %v, %v", err, db.updates)
		}
	}
}
