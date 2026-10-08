package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/go-jose/go-jose/v4"
)

// inlineSecretConfig 构造全部使用内联密钥的合法配置，作为对比基准。
func inlineSecretConfig(t *testing.T) *Config {
	t.Helper()
	c := Defaults()
	g := &c.Static.Auth.Gateway
	g.CookieSecretEnv, g.StorageKeyEnv, g.SigningEnv = "", "", ""
	g.CookieSecret = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	g.StorageKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	raw, err := json.Marshal(jose.JSONWebKey{Key: key, KeyID: "test", Algorithm: "EdDSA", Use: "sig"})
	if err != nil {
		t.Fatal("cannot generate fixture JWK")
	}
	g.SigningJWKS = &SigningJWKS{Keys: []json.RawMessage{raw}}
	c.Static.Storage.PasswordEnv, c.Static.Storage.Password = "", "test-db-password"
	c.App.Auth.OidcClient.Secret = "test-oidc-secret"
	c.App.MediaWiki.Bridge.Secret = strings.Repeat("b", 32)
	return c
}

func TestResolveSecretSources(t *testing.T) {
	t.Setenv("GATEWAY_TEST_SECRET", "env-value")
	t.Setenv("GATEWAY_TEST_EMPTY", "")
	t.Setenv("GATEWAY_TEST_PLACEHOLDER", "FILL_SECRET")
	t.Setenv("GATEWAY_TEST_UNSET", "")
	if err := os.Unsetenv("GATEWAY_TEST_UNSET"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, direct, env, want string
		required, invalid       bool
	}{
		{"direct", "direct-value", "", "direct-value", true, false},
		{"environment", "", "GATEWAY_TEST_SECRET", "env-value", true, false},
		{"conflict", "confidential-value", "GATEWAY_TEST_SECRET", "", true, true},
		{"required missing", "", "", "", true, true},
		{"optional absent", "", "", "", false, false},
		{"configured environment empty", "", "GATEWAY_TEST_EMPTY", "", false, true},
		{"configured environment absent", "", "GATEWAY_TEST_UNSET", "", false, true},
		{"placeholder direct", "FILL_SECRET", "", "", true, true},
		{"placeholder environment", "", "GATEWAY_TEST_PLACEHOLDER", "", true, true},
		{"whitespace preserved", " secret with spaces ", "", " secret with spaces ", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, err := resolveSecret("test.secret", tc.direct, tc.env, "", tc.required)
			if (err != nil) != tc.invalid {
				t.Fatal("unexpected secret-source validation result")
			}
			if err == nil && value != tc.want {
				t.Fatal("credential changed during resolution")
			}
			if err != nil && (!strings.Contains(err.Error(), "test.secret") || strings.Contains(err.Error(), "confidential-value")) {
				t.Fatal("error lacks field context or exposes credentials")
			}
		})
	}
}

func TestDefaultSecretEnvironmentFallback(t *testing.T) {
	t.Setenv("GATEWAY_TEST_DEFAULT", "fallback-value")
	// 未提供直接值与 Env 名称时使用默认 Env 名称。
	value, err := resolveSecret("test.secret", "", "", "GATEWAY_TEST_DEFAULT", true)
	if err != nil || value != "fallback-value" {
		t.Fatal("default environment name was not used as fallback")
	}
	// 提供直接值时默认 Env 名称不生效，避免与直接值冲突。
	if value, err = resolveSecret("test.secret", "direct-value", "", "GATEWAY_TEST_DEFAULT", true); err != nil || value != "direct-value" {
		t.Fatal("direct value did not take precedence over default environment name")
	}
}

func TestInlineAndEnvironmentSecretsEquivalent(t *testing.T) {
	inline := inlineSecretConfig(t)
	if err := inline.resolveSecrets(); err != nil {
		t.Fatal(err)
	}
	environment := inlineSecretConfig(t)
	g := &environment.Static.Auth.Gateway
	for i, pair := range []struct{ direct, env *string }{
		{&g.CookieSecret, &g.CookieSecretEnv}, {&g.StorageKey, &g.StorageKeyEnv},
		{&environment.Static.Storage.Password, &environment.Static.Storage.PasswordEnv},
		{&environment.App.Auth.OidcClient.Secret, &environment.App.Auth.OidcClient.SecretEnv},
		{&environment.App.MediaWiki.Bridge.Secret, &environment.App.MediaWiki.Bridge.SecretEnv},
	} {
		name := "GATEWAY_TEST_EQUIVALENT_" + string(rune('A'+i))
		t.Setenv(name, *pair.direct)
		*pair.direct, *pair.env = "", name
	}
	raw, err := json.Marshal(g.SigningJWKS)
	if err != nil {
		t.Fatal("cannot serialize fixture JWKS")
	}
	t.Setenv("GATEWAY_TEST_JWKS", string(raw))
	g.SigningJWKS, g.SigningEnv = nil, "GATEWAY_TEST_JWKS"
	if err = environment.resolveSecrets(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inline.Secrets, environment.Secrets) {
		t.Fatal("secret sources produce different resolved credentials")
	}
}

func TestInvalidSecretFormatsAndAtomicResolution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"cookie invalid", func(c *Config) { c.Static.Auth.Gateway.CookieSecret = "private-invalid-cookie" }},
		{"storage invalid", func(c *Config) { c.Static.Auth.Gateway.StorageKey = "private-invalid-storage" }},
		{"bridge short", func(c *Config) { c.App.MediaWiki.Bridge.Secret = "private-short-bridge" }},
		{"JWKS empty", func(c *Config) { c.Static.Auth.Gateway.SigningJWKS.Keys = nil }},
		{"JWKS malformed", func(c *Config) {
			c.Static.Auth.Gateway.SigningJWKS.Keys[0] = json.RawMessage(`{"kty":"unknown","d":"private-invalid-jwk"}`)
		}},
		{"JWKS conflict", func(c *Config) { c.Static.Auth.Gateway.SigningEnv = "GATEWAY_TEST_JWKS" }},
		{"default Env conflict", func(c *Config) {
			c.Static.Auth.Gateway.CookieSecretEnv = defaultCookieSecretEnv
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := inlineSecretConfig(t)
			// 预置旧值，验证解析失败后不会残留部分凭据。
			c.Secrets.DBPassword = "previous"
			tc.change(c)
			err := c.resolveSecrets()
			if err == nil {
				t.Fatal("invalid credentials accepted")
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("validation error exposes credential")
			}
			if c.Secrets.DBPassword != "previous" || c.Secrets.Cookie != nil {
				t.Fatal("failed resolution published partial credentials")
			}
		})
	}
}

func TestPreRegisteredClientSecretRequirements(t *testing.T) {
	for _, method := range []string{"none", "", "client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			c := inlineSecretConfig(t)
			c.Static.Auth.Gateway.Registration.Clients = []Client{{ID: "test-client", AuthMethod: method}}
			required := method == "client_secret_basic" || method == "client_secret_post"
			if err := c.resolveSecrets(); (err != nil) != required {
				t.Fatal("incorrect client secret requirement")
			}
			c.Static.Auth.Gateway.Registration.Clients[0].Secret = "client-secret"
			if err := c.resolveSecrets(); err != nil {
				t.Fatal(err)
			}
			if c.Secrets.ClientSecrets["test-client"] != "client-secret" {
				t.Fatal("client credential not resolved")
			}
			t.Setenv("GATEWAY_TEST_REGISTERED_SECRET", "env-client-secret")
			client := &c.Static.Auth.Gateway.Registration.Clients[0]
			client.Secret, client.SecretEnv = "", "GATEWAY_TEST_REGISTERED_SECRET"
			if err := c.resolveSecrets(); err != nil {
				t.Fatal(err)
			}
			if c.Secrets.ClientSecrets["test-client"] != "env-client-secret" {
				t.Fatal("environment client credential not resolved")
			}
			client.Secret = "private-conflicting-client-secret"
			if err := c.resolveSecrets(); err == nil || strings.Contains(err.Error(), client.Secret) {
				t.Fatal("client source conflict accepted or leaked")
			}
		})
	}
}

func TestEnvironmentSecretFormatValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config, string)
	}{
		{"cookie", func(c *Config, name string) {
			c.Static.Auth.Gateway.CookieSecret = ""
			c.Static.Auth.Gateway.CookieSecretEnv = name
		}},
		{"storage", func(c *Config, name string) {
			c.Static.Auth.Gateway.StorageKey = ""
			c.Static.Auth.Gateway.StorageKeyEnv = name
		}},
		{"bridge", func(c *Config, name string) {
			c.App.MediaWiki.Bridge.Secret = ""
			c.App.MediaWiki.Bridge.SecretEnv = name
		}},
		{"signing", func(c *Config, name string) {
			c.Static.Auth.Gateway.SigningJWKS = nil
			c.Static.Auth.Gateway.SigningEnv = name
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GATEWAY_TEST_INVALID_FORMAT", "private-invalid")
			c := inlineSecretConfig(t)
			tc.set(c, "GATEWAY_TEST_INVALID_FORMAT")
			if err := c.resolveSecrets(); err == nil || strings.Contains(err.Error(), "private-invalid") {
				t.Fatal("invalid environment credential accepted or leaked")
			}
		})
	}
}

func TestPublicSigningKeyRejectedForBothSources(t *testing.T) {
	for _, fromEnv := range []bool{false, true} {
		c := inlineSecretConfig(t)
		var key map[string]any
		if err := json.Unmarshal(c.Static.Auth.Gateway.SigningJWKS.Keys[0], &key); err != nil {
			t.Fatal("invalid test fixture")
		}
		// 移除私钥参数 d，构造仅有公钥的 JWK。
		delete(key, "d")
		raw, err := json.Marshal(key)
		if err != nil {
			t.Fatal("invalid test fixture")
		}
		c.Static.Auth.Gateway.SigningJWKS.Keys[0] = raw
		if fromEnv {
			raw, err = json.Marshal(c.Static.Auth.Gateway.SigningJWKS)
			if err != nil {
				t.Fatal("invalid test fixture")
			}
			t.Setenv("GATEWAY_TEST_PUBLIC_JWKS", string(raw))
			c.Static.Auth.Gateway.SigningJWKS, c.Static.Auth.Gateway.SigningEnv = nil, "GATEWAY_TEST_PUBLIC_JWKS"
		}
		if err := c.resolveSecrets(); err == nil {
			t.Fatal("public signing key accepted")
		}
	}
}

func TestSecretSchema(t *testing.T) {
	for _, tc := range []struct {
		root reflect.Type
		path []string
	}{
		{reflect.TypeFor[AppFile](), []string{"auth", "oidc_client", "clientSecret"}},
		{reflect.TypeFor[AppFile](), []string{"mediawiki", "serviceBridge", "secret"}},
		{reflect.TypeFor[ServerFile](), []string{"storage", "password"}},
		{reflect.TypeFor[ServerFile](), []string{"auth", "gateway", "cookieSecret"}},
		{reflect.TypeFor[ServerFile](), []string{"auth", "gateway", "storageEncryptionKey"}},
		{reflect.TypeFor[ServerFile](), []string{"auth", "gateway", "signingJwks"}},
	} {
		node := schema(tc.root)
		for _, field := range tc.path {
			node = node["properties"].(map[string]any)[field].(map[string]any)
		}
		if node["writeOnly"] != true {
			t.Fatal("inline secret is not writeOnly")
		}
	}
	if got := schema(reflect.TypeFor[*SigningJWKS]())["type"]; !reflect.DeepEqual(got, []string{"object", "null"}) {
		t.Fatal("JWKS schema must accept an object or null")
	}
	client := schema(reflect.TypeFor[Client]())["properties"].(map[string]any)
	if client["clientSecret"].(map[string]any)["writeOnly"] != true {
		t.Fatal("pre-registered client secret is not writeOnly")
	}
}
