package application

import (
	"context"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
	"testing"
	"time"
)

func TestCustomSchemeReview(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		valid   bool
		blocked bool
	}{
		{"workbuddy://workbuddy/mcp/custom-mcp%3Aisekaiwiki/oauth/callback", true, true},
		{"com.example.app:/callback", true, false}, {"https://client.example/callback", true, false},
		{"http://127.0.0.1:1234/callback", true, false}, {"http://remote.example/callback", false, false},
		{"javascript:alert(1)", false, false}, {"data:text/html,x", false, false}, {"file:///tmp/x", false, false},
		{"workbuddy://user@host/callback", false, false}, {"workbuddy://host/callback#x", false, false}, {"workbuddy://host/%zz", false, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c := &Client{Source: "dcr", Redirects: []string{tc.raw}}
			err := new(Service).validateClient(c, true)
			if (err == nil) != tc.valid {
				t.Fatalf("validation: %v", err)
			}
			if tc.valid && (registrationTrust(c) == domain.TrustBlocked) != tc.blocked {
				t.Fatal("review classification")
			}
		})
	}
	c := &Client{Source: "cimd", Redirects: []string{"https://client.example/callback", "workbuddy://host/callback"}}
	if registrationTrust(c) != domain.TrustBlocked {
		t.Fatal("mixed redirects not blocked")
	}
}

type reviewDB struct{ Database }

func (d *reviewDB) WithContext(context.Context) Database     { return d }
func (d *reviewDB) Transaction(f func(Database) error) error { return f(d) }
func (d *reviewDB) Table(string) Database                    { return d }
func (d *reviewDB) Where(any, ...any) Database               { return d }
func (d *reviewDB) Update(string, any) Database              { return d }
func (d *reviewDB) Delete(any) Database                      { return d }
func (d *reviewDB) Error() error                             { return nil }

type reviewStore struct {
	RecordStore
	db       reviewDB
	previous storedClient
	level    domain.TrustLevel
}

func (s *reviewStore) Database() Database { return &s.db }
func (s *reviewStore) Get(_ Database, _ string, key string, v any, _ bool) (persistence.Row, error) {
	if key == "@registry" {
		return persistence.Row{}, nil
	}
	*v.(*storedClient) = s.previous
	return persistence.Row{TrustLevel: string(s.level)}, nil
}
func (s *reviewStore) Put(_ Database, _ string, _ string, v any, row persistence.Row) error {
	s.previous = v.(storedClient)
	s.level = domain.TrustLevel(row.TrustLevel)
	return nil
}
func TestCIMDReviewRefresh(t *testing.T) {
	c := Client{ID: "https://app.example/client.json", Source: "cimd", Redirects: []string{"workbuddy://host/callback"}, FetchedAt: time.Now()}
	svc := &Service{Config: config.Defaults()}
	if err := svc.validateClient(&c, true); err != nil {
		t.Fatal(err)
	}
	for _, level := range []domain.TrustLevel{domain.TrustBlocked, domain.TrustUnknown, domain.TrustKnown, domain.TrustTrusted} {
		store := &reviewStore{previous: storedClient{Source: "cimd", Fingerprint: c.Fingerprint}, level: level}
		svc.Store = store
		if err := svc.updateClient(context.Background(), &c, "active"); err != nil {
			t.Fatal(err)
		}
		if c.TrustLevel != level {
			t.Fatal("refresh overwrote review", level)
		}
		c.Name = "Changed metadata"
		if err := svc.validateClient(&c, true); err != nil {
			t.Fatal(err)
		}
		if err := svc.updateClient(context.Background(), &c, "active"); err != nil {
			t.Fatal(err)
		}
		if c.TrustLevel != domain.TrustBlocked {
			t.Fatal("changed fingerprint not reviewed")
		}
		c.Name = ""
		if err := svc.validateClient(&c, true); err != nil {
			t.Fatal(err)
		}
	}
}
