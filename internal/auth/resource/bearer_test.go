package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

type bearerDB struct{ application.Database }

func (d *bearerDB) WithContext(context.Context) application.Database { return d }

type bearerStore struct {
	application.RecordStore
	cause error
}

func (s *bearerStore) Database() application.Database { return new(bearerDB) }
func (s *bearerStore) Get(application.Database, string, string, any, bool) (persistence.Row, error) {
	return persistence.Row{}, s.cause
}

type bearerDecoder struct{}

func (bearerDecoder) Decrypt(string) (string, error) { return "token:account", nil }

func TestBearerInfrastructureDoesNotChallenge(t *testing.T) {
	for _, test := range []struct {
		err       error
		status    int
		challenge bool
	}{
		{application.ErrRecordNotFound, 401, true},
		{errors.New("secret backend SQL"), 500, false},
		{&failure.Error{Code: failure.Unavailable, Message: "safe", Retryable: true}, 503, false},
	} {
		s := &Service{Service: &application.Service{Config: config.Defaults(), Store: &bearerStore{cause: test.err}, Decoder: bearerDecoder{}}}
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.Header.Set("Authorization", "Bearer test")
		w := httptest.NewRecorder()
		s.Bearer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("failed bearer reached handler") })).ServeHTTP(w, r)
		if w.Code != test.status || (w.Header().Get("WWW-Authenticate") != "") != test.challenge || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("wrong bearer response: %d %s", w.Code, w.Body.String())
		}
	}
}

type apiKeyDB struct{ application.Database }

func (d *apiKeyDB) WithContext(context.Context) application.Database { return d }

// keyStore 支撑 sk- 密钥校验链的内存仓库。
type keyStore struct {
	application.RecordStore
	key domain.APIKey
	row persistence.Row
}

func (s *keyStore) Database() application.Database { return &apiKeyDB{} }
func (s *keyStore) Get(_ application.Database, kind, key string, value any, _ bool) (persistence.Row, error) {
	switch kind {
	case "api_keys":
		if key != s.key.ID {
			return persistence.Row{}, application.ErrRecordNotFound
		}
		*value.(*domain.APIKey) = s.key
		return s.row, nil
	case "accounts":
		*value.(*application.Account) = application.Account{ID: s.key.AccountID}
		return persistence.Row{State: "active"}, nil
	}
	return persistence.Row{}, application.ErrRecordNotFound
}

func TestBearerAPIKeyDispatch(t *testing.T) {
	secret := "q83q0xkmlzvhm9t6pqcgp1ivmno9mg7cp1kw508otfovp0p08x0"
	digest := sha256.Sum256([]byte(secret))
	store := &keyStore{key: domain.APIKey{ID: "keyid12345678901", AccountID: "account", Name: "k", Scopes: []string{"mcp:access", "wiki:read"}, Identity: domain.Identity{UserID: 3, UserToken: "t"}, SecretHash: hex.EncodeToString(digest[:])}, row: persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}}
	cfg := config.Defaults()
	s := &Service{Service: &application.Service{Config: cfg, Store: store}}
	plaintext := "sk-keyid12345678901" + secret
	for _, test := range []struct {
		name, header, value string
		status              int
		reached             bool
	}{
		{"bearer", "Authorization", "Bearer " + plaintext, 200, true},
		{"api-key header", "X-API-Key", plaintext, 200, true},
		{"forged", "X-API-Key", "sk-keyid12345678901" + strings.Repeat("x", 43), 401, false},
		{"unknown", "X-API-Key", "sk-unknownid1234567" + secret, 401, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/mcp", nil)
			r.Header.Set(test.header, test.value)
			w := httptest.NewRecorder()
			reached := false
			s.Bearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				p, _ := PrincipalFromContext(r.Context())
				if p == nil || p.AuthKind != domain.AuthKindAPIKey || p.OAuthClientID != "api-key" || p.GrantID != "keyid12345678901" || p.WikiUserID != 3 {
					t.Fatalf("principal: %+v", p)
				}
			})).ServeHTTP(w, r)
			if w.Code != test.status || reached != test.reached {
				t.Fatalf("status=%d reached=%v", w.Code, reached)
			}
		})
	}
	// 禁用 API key 时 sk- 令牌一律视为 invalid_token。
	cfg.Static.Auth.Gateway.APIKeys.Enabled = false
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("X-API-Key", plaintext)
	w := httptest.NewRecorder()
	s.Bearer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("disabled key reached handler") })).ServeHTTP(w, r)
	if w.Code != 401 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "invalid_token") {
		t.Fatalf("disabled response: %d %s", w.Code, w.Header().Get("WWW-Authenticate"))
	}
}

