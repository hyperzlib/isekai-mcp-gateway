package oidcclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"

	"isekai-mcp-gateway/internal/config"
)

func TestOIDCLoginValidation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	var token string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": server.URL, "authorization_endpoint": server.URL + "/authorize", "token_endpoint": server.URL + "/token", "jwks_uri": server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"EdDSA"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: public, KeyID: "test", Algorithm: "EdDSA", Use: "sig"}}})
		case "/token":
			if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") != "verifier" || r.Form.Get("redirect_uri") != "https://gateway.example/auth/oidc_client/callback" {
				t.Error("token exchange omitted bound PKCE verifier or callback")
				http.Error(w, "bad request", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "upstream-only", "token_type": "Bearer", "id_token": token})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := config.Defaults()
	c.App.Auth.OidcClient.Issuer, c.App.Auth.OidcClient.ClientID = server.URL, "gateway"
	c.App.Auth.OidcClient.RedirectURI = "https://gateway.example/auth/oidc_client/callback"
	c.Secrets.OidcClientSecret = "test-secret"
	client, err := New(context.Background(), c, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(client.AuthorizationURL("state", "nonce", "verifier"))
	if u.Query().Get("state") != "state" || u.Query().Get("nonce") != "nonce" || u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") != oauth2.S256ChallengeFromVerifier("verifier") {
		t.Fatal("authorization request lost state, nonce or S256 binding")
	}
	// 覆盖 issuer、audience、nonce、过期与签名不匹配等校验分支
	for _, test := range []struct {
		name, issuer, audience, nonce string
		expired, badSignature, valid  bool
	}{
		{"valid", server.URL, "gateway", "nonce", false, false, true},
		{"wrong issuer", "https://wrong.example", "gateway", "nonce", false, false, false},
		{"wrong audience", server.URL, "other", "nonce", false, false, false},
		{"wrong nonce", server.URL, "gateway", "other", false, false, false},
		{"expired", server.URL, "gateway", "nonce", true, false, false},
		{"bad signature", server.URL, "gateway", "nonce", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 按用例切换签名密钥与过期时间，构造对应的 ID token
			key, expires := private, time.Now().Add(time.Hour)
			if test.badSignature {
				key = wrongKey
			}
			if test.expired {
				expires = time.Now().Add(-time.Hour)
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: jose.JSONWebKey{Key: key, KeyID: "test"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			claims, _ := json.Marshal(map[string]any{"iss": test.issuer, "aud": test.audience, "sub": "wiki-subject", "nonce": test.nonce, "exp": expires.Unix(), "iat": time.Now().Unix()})
			signed, err := signer.Sign(claims)
			if err != nil {
				t.Fatal(err)
			}
			token, err = signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			subject, err := client.Authenticate(context.Background(), "code", "verifier", "nonce")
			if (err == nil) != test.valid || test.valid && subject != "wiki-subject" {
				t.Fatalf("subject=%q, error=%v", subject, err)
			}
		})
	}
}
