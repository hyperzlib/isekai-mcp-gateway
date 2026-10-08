package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/persistence"
)

type faultStore struct {
	*logoutStore
	kind     string
	cause    error
	state    string
	database Database
	reads    []string
}

func newFaultStore(kind string, cause error) *faultStore {
	return &faultStore{logoutStore: &logoutStore{db: new(logoutDB), grant: Grant{ID: "grant", AccountID: "account", ClientID: "client", Fingerprint: "fingerprint", SessionID: "session", Resource: "resource"}}, kind: kind, cause: cause}
}
func (s *faultStore) Database() Database {
	if s.database != nil {
		return s.database
	}
	return s.logoutStore.Database()
}
func (s *faultStore) Get(db Database, kind, key string, value any, lock bool) (persistence.Row, error) {
	// 记录读取顺序，并按 kind 注入故障；其余读取委托给底层存储。
	s.reads = append(s.reads, kind)
	if kind == s.kind && s.cause != nil {
		return persistence.Row{}, s.cause
	}
	var row persistence.Row
	switch kind {
	case "access_tokens", "refresh_tokens":
		*value.(*Token) = Token{GrantID: "grant", Subject: "account", ClientID: "client", Resource: "resource", IdleExpiresAt: time.Now().Add(time.Hour)}
		row = persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}
	case "login_transactions":
		*value.(*LoginTransaction) = LoginTransaction{Browser: "browser"}
		row = persistence.Row{State: "active", ExpiresAt: time.Now().Add(time.Hour)}
	default:
		var err error
		row, err = s.logoutStore.Get(db, kind, key, value, lock)
		if err != nil {
			return row, err
		}
	}
	if kind == s.kind && s.state != "" {
		row.State = s.state
	}
	return row, nil
}

func TestAuthorizationReadsPreserveInfrastructure(t *testing.T) {
	// 基础设施故障不得被降级为 invalid_token，且错误信息不得泄露内部细节。
	cause := errors.New("secret query and private payload")
	for _, kind := range []string{"access_tokens", "grants", "clients", "accounts", "sessions"} {
		t.Run(kind, func(t *testing.T) {
			store := newFaultStore(kind, cause)
			s := &Service{Store: store}
			_, _, _, err := s.AccessByID(store.Database(), "token", "account")
			if !errors.Is(err, cause) || errors.Is(err, ErrInvalidToken) || !failure.IsInfrastructure(err) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("infrastructure failure masked: %v", err)
			}
			store.cause, store.state = nil, "revoked"
			_, _, _, err = s.AccessByID(store.Database(), "token", "account")
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("revoked %s accepted: %v", kind, err)
			}
			store.cause, store.state = ErrRecordNotFound, ""
			_, _, _, err = s.AccessByID(store.Database(), "token", "account")
			if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("absent %s masked incorrectly: %v", kind, err)
			}
		})
	}
}

func TestRefreshAndRevocationFailures(t *testing.T) {
	// 刷新与吊销的上游故障必须原样保留，不能被误映射为无效令牌。
	cause := errors.New("secret encrypted payload corrupt")
	store := newFaultStore("refresh_tokens", cause)
	s := &Service{Store: store}
	ctx := context.WithValue(context.Background(), RequestKey{}, RequestInfo{ClientID: "client", Resource: "resource"})
	_, err := s.TokenRequestByRefreshToken(ctx, "token")
	var oauth *domain.OAuthError
	if !errors.Is(err, cause) || errors.As(err, &oauth) {
		t.Fatal("refresh failure became invalid_grant")
	}
	_, _, err = s.GetRefreshTokenInfo(ctx, "client", "token")
	if !errors.Is(err, cause) || errors.Is(err, domain.ErrInvalidRefreshToken) {
		t.Fatal("refresh lookup failure became invalid refresh token")
	}
	store.kind, store.reads = "access_tokens", nil
	err = s.RevokeToken(ctx, "token", "account", "client")
	if !errors.Is(err, cause) || len(store.reads) != 1 {
		t.Fatal("revocation retried a failed access lookup as a refresh lookup")
	}
}

type transactionFaultDB struct {
	*logoutDB
	cause error
}

func (d *transactionFaultDB) WithContext(context.Context) Database      { return d }
func (d *transactionFaultDB) Transaction(fn func(Database) error) error { return fn(d) }
func (d *transactionFaultDB) Table(string) Database                     { return d }
func (d *transactionFaultDB) Where(any, ...any) Database                { return d }
func (d *transactionFaultDB) Update(string, any) Database               { return d }
func (d *transactionFaultDB) Error() error                              { return d.cause }

func TestLoginTransactionFailures(t *testing.T) {
	// 登录事务的存储故障应返回 500，缺失记录应返回 400。
	cause := errors.New("secret database fault")
	for _, write := range []bool{false, true} {
		store := newFaultStore("login_transactions", cause)
		if write {
			store.cause = nil
			store.database = &transactionFaultDB{logoutDB: new(logoutDB), cause: cause}
		}
		s := &Service{Store: store, Config: config.Defaults()}
		_, err := s.CompleteLogin(context.Background(), "state", "code", "browser", false)
		var flow *FlowError
		if !errors.As(err, &flow) || flow.Status != 500 || !errors.Is(err, cause) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("login storage error became expiration: %v", err)
		}
	}
	store := newFaultStore("login_transactions", ErrRecordNotFound)
	_, err := (&Service{Store: store}).CompleteLogin(context.Background(), "state", "code", "browser", false)
	var flow *FlowError
	if !errors.As(err, &flow) || flow.Status != 400 {
		t.Fatal("missing login transaction no longer rejected")
	}
	for _, test := range []struct {
		cause  error
		status int
	}{{failure.New(failure.Upstream, "safe", "oidc", nil), 502}, {failure.Dependency("oidc", context.DeadlineExceeded), 504}} {
		if flow := loginError("login", test.cause).(*FlowError); flow.Status != test.status || !errors.Is(flow, test.cause) {
			t.Fatal("login dependency mapping incorrect")
		}
	}
}
