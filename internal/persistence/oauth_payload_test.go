package persistence

import (
	"bytes"
	"encoding/json"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/security"
	"reflect"
	"strings"
	"testing"
)

func TestOAuthPayloadRoundTrip(t *testing.T) {
	vault, err := security.NewVault([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{Vault: vault}
	tests := []struct {
		kind    string
		value   any
		secrets []string
	}{
		{"clients", domain.StoredClient{Metadata: domain.Client{ID: "client", Name: "App"}, SecretHash: "private-secret-hash"}, []string{"private-secret-hash"}},
		{"accounts", domain.Account{ID: "account", WikiUserID: 9007199254740993, Username: "Alice"}, nil},
		{"sessions", domain.Session{ID: "private-session", Browser: "private-browser", Identity: domain.Identity{UserID: 9007199254740993, UserToken: "private-user-token"}}, []string{"private-session", "private-browser", "private-user-token"}},
		{"login_transactions", domain.LoginTransaction{ID: "private-state", Browser: "private-browser", Nonce: "private-nonce", Verifier: "private-verifier", Snapshot: []byte("private-snapshot")}, []string{"private-state", "private-browser", "private-nonce", "private-verifier"}},
		{"interactions", domain.Interaction{Browser: "private-browser", SessionID: "private-session", Request: domain.AuthorizationRequest{State: "private-state", Nonce: "private-nonce", IDTokenHint: "private-hint", RequestParam: "private-request", ClientID: "client"}}, []string{"private-browser", "private-session", "private-state", "private-nonce", "private-hint", "private-request"}},
		{"client_trusts", domain.ClientTrust{AccountID: "account", ClientID: "client"}, nil},
		{"grants", domain.Grant{ID: "grant", ClientID: "client", SessionID: "private-session"}, []string{"private-session"}},
		{"authorization_codes", domain.Code{InteractionID: "interaction"}, nil},
		{"access_tokens", domain.Token{ID: "private-access", Subject: "account"}, []string{"private-access"}},
		{"refresh_tokens", domain.Token{ID: "private-refresh", Subject: "account"}, []string{"private-refresh"}},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			row, err := s.encode(tc.kind, "key", tc.value, Row{State: "registry"})
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(row.Payload) {
				t.Fatal("plaintext not JSON")
			}
			for _, secret := range tc.secrets {
				if bytes.Contains(row.Payload, []byte(secret)) {
					t.Fatal("plaintext credential")
				}
			}
			out := reflect.New(reflect.TypeOf(tc.value))
			if err := s.OpenRecord(row, out.Interface(), tc.kind); err != nil {
				t.Fatal(err)
			}
			originalJSON, _ := json.Marshal(tc.value)
			recoveredJSON, _ := json.Marshal(out.Elem().Interface())
			if !bytes.Equal(originalJSON, recoveredJSON) {
				t.Fatalf("round trip: %#v", out.Elem().Interface())
			}
			if len(row.EncryptedPayload) > 0 {
				row.ID = "another-row"
				if s.OpenRecord(row, out.Interface(), tc.kind) == nil {
					t.Fatal("cross-row ciphertext accepted")
				}
				row.ID = security.Hash("key")
				other := "sessions"
				if tc.kind == other {
					other = "clients"
				}
				if s.OpenRecord(row, out.Interface(), other) == nil {
					t.Fatal("cross-table ciphertext accepted")
				}
			}
		})
	}
	row, err := s.encode("clients", "empty", domain.StoredClient{}, Row{State: "registry"})
	if err != nil {
		t.Fatal(err)
	}
	if len(row.EncryptedPayload) != 0 {
		t.Fatal("empty secret should not require ciphertext")
	}
}
