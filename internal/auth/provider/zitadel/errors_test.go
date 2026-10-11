package zitadel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/persistence"
	"isekai-mcp-gateway/internal/security"
)

type endpointDB struct {
	application.Database
	count int64
	cause error
}

func (d *endpointDB) WithContext(context.Context) application.Database      { return d }
func (d *endpointDB) Transaction(fn func(application.Database) error) error { return fn(d) }
func (d *endpointDB) Order(any) application.Database                        { return d }
func (d *endpointDB) Lock() application.Database                            { return d }
func (d *endpointDB) Find(any) application.Database                         { return d }
func (d *endpointDB) Table(string) application.Database                     { return d }
func (d *endpointDB) Where(any, ...any) application.Database                { return d }
func (d *endpointDB) Update(string, any) application.Database               { return d }
func (d *endpointDB) Count(n *int64) application.Database                   { *n = d.count; return d }
func (d *endpointDB) Error() error                                          { return d.cause }

type endpointStore struct {
	application.RecordStore
	db              *endpointDB
	cause           error
	method          string
	blocked         bool
	registryFailure bool
}

func (s *endpointStore) Database() application.Database { return s.db }
func (s *endpointStore) Get(_ application.Database, kind, key string, value any, _ bool) (persistence.Row, error) {
	if kind == "clients" && key == "@registry" {
		if s.registryFailure {
			return persistence.Row{}, s.cause
		}
		return persistence.Row{}, nil
	}
	if kind == "clients" {
		level := domain.TrustUnknown
		if s.blocked {
			level = domain.TrustBlocked
		}
		data, _ := json.Marshal(map[string]any{
			"Metadata":       application.Client{ID: "client", TrustLevel: level, TokenAuthMethod: s.method, Grants: []string{"authorization_code", "refresh_token"}, Responses: []string{"code"}, Scope: "mcp:access offline_access"},
			"ResourceClient": true, "SecretHash": security.Hash("test-password"), "Source": "pre",
		})
		if err := json.Unmarshal(data, value); err != nil {
			return persistence.Row{}, err
		}
		value.(*domain.StoredClient).Metadata.TrustLevel = level
		return persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	return persistence.Row{}, s.cause
}
func (s *endpointStore) Put(application.Database, string, string, any, persistence.Row) error {
	return s.cause
}

func TestRegistrationFailureResponses(t *testing.T) {
	cause := errors.New("secret SQL https://private/?password=x")
	for _, test := range []struct {
		name, body         string
		capacity, registry bool
		status             int
		code               string
	}{
		{"validation", `{"redirect_uris":["javascript:secret"]}`, false, false, 400, "invalid_client_metadata"},
		{"registry", `{"redirect_uris":["https://client.example/callback"]}`, false, true, 500, "server_error"},
		{"write", `{"redirect_uris":["https://client.example/callback"]}`, false, false, 500, "server_error"},
		{"capacity", `{"redirect_uris":["https://client.example/callback"]}`, true, false, 503, "temporarily_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := config.Defaults()
			store := &endpointStore{db: new(endpointDB), cause: cause, registryFailure: test.registry}
			if test.capacity {
				store.db.count = int64(c.Static.Auth.Gateway.Registration.MaxClients)
			}
			s := &Service{Service: &application.Service{Config: c, Store: store}}
			w := httptest.NewRecorder()
			s.Registration(w, httptest.NewRequest("POST", "/oauth/register", strings.NewReader(test.body)))
			if w.Code != test.status || !strings.Contains(w.Body.String(), test.code) || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func testProvider(t *testing.T, store *endpointStore) *Service {
	t.Helper()
	c := config.Defaults()
	c.Static.Auth.Gateway.Issuer = "http://127.0.0.1:3000"
	c.Static.Auth.Gateway.Resource = c.Static.Auth.Gateway.Issuer + "/mcp"
	keys, err := security.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(keys.SigningJWKS)
	if err != nil {
		t.Fatal(err)
	}
	c.Secrets.Signing, c.Secrets.Storage = string(b), make([]byte, 32)
	s, err := New(c, store, new(outbound.Manager))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRealProviderDoesNotMaskStorageFailures(t *testing.T) {
	cause := errors.New("secret SQL and upstream payload")
	for _, endpoint := range []string{"refresh", "introspect", "userinfo", "revoke"} {
		t.Run(endpoint, func(t *testing.T) {
			store := &endpointStore{db: new(endpointDB), cause: cause, method: "client_secret_basic"}
			if endpoint == "refresh" {
				store.method = "none"
			}
			s := testProvider(t, store)
			raw, err := s.Provider.Crypto().Encrypt("token:account")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{"token": {raw}}
			path := "/oauth/" + endpoint
			if endpoint == "refresh" {
				path = "/oauth/token"
				form = url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"refresh"}, "client_id": {"client"}, "resource": {s.Config.Static.Auth.Gateway.Resource}}
			}
			if endpoint == "revoke" {
				form.Set("token_type_hint", "access_token")
			}
			r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if endpoint == "userinfo" {
				r.Header.Set("Authorization", "Bearer "+raw)
			} else if endpoint != "refresh" {
				r.SetBasicAuth("client", "test-password")
			}
			r = r.WithContext(failure.NewRequest(r.Context()))
			w := httptest.NewRecorder()
			switch endpoint {
			case "refresh":
				s.TokenEndpoint(w, r)
			case "introspect":
				s.Introspect(w, r)
			case "userinfo":
				s.Userinfo(w, r)
			case "revoke":
				s.Revocation(w, r)
			}
			if w.Code != 500 || !strings.Contains(w.Body.String(), "server_error") || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "invalid_grant") {
				t.Fatalf("provider masked failure: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestProviderResponseRewriteFailsClosed(t *testing.T) {
	s := &Service{Service: &application.Service{Config: config.Defaults()}}
	for _, body := range []string{`null`, `{"id_token":"secret"`, `{"id_token":"secret","access_token":"ok"}`} {
		w := httptest.NewRecorder()
		s.providerResponseWith(w, httptest.NewRequest("POST", "/oauth/token", nil), true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "999")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(body))
		}))
		want := 500
		if strings.HasSuffix(body, `"ok"}`) {
			want = 200
		}
		if w.Code != want || strings.Contains(w.Body.String(), "secret") || w.Header().Get("Content-Length") != "" {
			t.Fatalf("unsafe rewrite: %d %s", w.Code, w.Body.String())
		}
	}
	unknown := providerError(context.Background(), errors.New("secret"))
	if !errors.Is(unknown, oidc.ErrServerError()) || strings.Contains(unknown.Error(), "secret") {
		t.Fatal("unknown provider error exposed")
	}
}

func TestRealProviderPreservesAbsentTokenSemantics(t *testing.T) {
	for _, endpoint := range []string{"refresh", "introspect", "userinfo", "revoke"} {
		t.Run(endpoint, func(t *testing.T) {
			store := &endpointStore{db: new(endpointDB), cause: application.ErrRecordNotFound, method: "client_secret_basic"}
			if endpoint == "refresh" {
				store.method = "none"
			}
			s := testProvider(t, store)
			raw, err := s.Provider.Crypto().Encrypt("missing:account")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{"token": {raw}}
			path, want := "/oauth/"+endpoint, 200
			switch endpoint {
			case "refresh":
				path, want = "/oauth/token", 400
				form = url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"missing"}, "client_id": {"client"}, "resource": {s.Config.Static.Auth.Gateway.Resource}}
			case "userinfo":
				want = 403
			case "revoke":
				form.Set("token_type_hint", "access_token")
			}
			r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if endpoint == "userinfo" {
				r.Header.Set("Authorization", "Bearer "+raw)
			} else if endpoint != "refresh" {
				r.SetBasicAuth("client", "test-password")
			}
			w := httptest.NewRecorder()
			switch endpoint {
			case "refresh":
				s.TokenEndpoint(w, r)
			case "introspect":
				s.Introspect(w, r)
			case "userinfo":
				s.Userinfo(w, r)
			case "revoke":
				s.Revocation(w, r)
			}
			if w.Code != want || strings.Contains(w.Body.String(), "server_error") {
				t.Fatalf("business failure became infrastructure: %d %s", w.Code, w.Body.String())
			}
			if endpoint == "introspect" && !strings.Contains(w.Body.String(), `"active":false`) {
				t.Fatal("absent token introspected as active")
			}
		})
	}
}

func TestBlockedClientEndpoints(t *testing.T) {
	cfg := config.Defaults()
	svc := &Service{Service: &application.Service{Config: cfg, Store: &endpointStore{db: new(endpointDB), blocked: true}}}
	svc.PageError = func(w http.ResponseWriter, r *http.Request, message string, status int) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		w.Write([]byte("<!doctype html><p>" + message + "</p>"))
	}
	for _, endpoint := range []string{"authorize", "token"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/oauth/"+endpoint, strings.NewReader(url.Values{"client_id": {"client"}, "grant_type": {"authorization_code"}, "resource": {cfg.Static.Auth.Gateway.Resource}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if endpoint == "authorize" {
			svc.Authorize(w, r)
		} else {
			svc.TokenEndpoint(w, r)
		}
		if w.Code != 400 || !strings.Contains(w.Body.String(), domain.BlockedClientDescription) {
			t.Fatalf("%s: %d %s", endpoint, w.Code, w.Body.String())
		}
		if endpoint == "authorize" && (!strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "<!doctype html>")) {
			t.Fatal("authorization error is not HTML")
		}
		if endpoint == "token" && !strings.Contains(w.Body.String(), "unauthorized_client") {
			t.Fatal("token error is not OAuth JSON")
		}
	}
}

func TestWorkBuddyRedirectCompatibility(t *testing.T) {
	raw := "workbuddy://workbuddy/mcp/custom-mcp%3Aisekaiwiki/oauth/callback"
	c := &Client{Client: &domain.Client{TokenAuthMethod: "none", Redirects: []string{raw}}}
	if !c.ValidRedirect(raw) {
		t.Fatal("registered redirect rejected")
	}
	if err := op.ValidateAuthReqRedirectURI(c, raw, oidc.ResponseTypeCode); err != nil {
		t.Fatal(err)
	}
	if c.ValidRedirect(raw + "/other") {
		t.Fatal("unregistered redirect accepted")
	}
}
