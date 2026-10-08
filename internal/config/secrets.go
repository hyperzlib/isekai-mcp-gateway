package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/security"
)

// 默认 Env 名称仅在既未提供直接值、也未显式指定 Env 时作为回退。
const (
	defaultCookieSecretEnv = "GATEWAY_OAUTH_COOKIE_SECRET"
	defaultStorageKeyEnv   = "GATEWAY_OAUTH_STORAGE_KEY"
	defaultSigningEnv      = "GATEWAY_OAUTH_SIGNING_JWKS"
	defaultDBPasswordEnv   = "GATEWAY_DB_PASSWORD"
)

// resolveSecret accepts exactly one configured source, without trimming credentials.
// defaultEnv applies only when neither a direct value nor an explicit Env name is set.
func resolveSecret(field, inline, envName, defaultEnv string, required bool) (string, error) {
	// 直接值与 Env 名称互斥，避免来源歧义。
	if inline != "" && envName != "" {
		return "", &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: field + ": direct value and Env field cannot both be set", Op: "config.secret", Field: field}
	}
	if inline == "" && envName == "" {
		envName = defaultEnv
	}
	value := inline
	if envName != "" {
		var ok bool
		value, ok = os.LookupEnv(envName)
		if !ok || value == "" {
			return "", &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: field + ": configured environment variable is missing or empty", Op: "config.secret", Field: field}
		}
	}
	// 拒绝脚手架生成的占位符，防止未填写的密钥被当作真实值使用。
	if strings.HasPrefix(value, "FILL_") {
		return "", &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: field + ": placeholder value is not allowed", Op: "config.secret", Field: field}
	}
	if required && value == "" {
		return "", &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: field + ": a direct value or Env field is required", Op: "config.secret", Field: field}
	}
	return value, nil
}

func (c *Config) resolveSecrets() error {
	g, storage, oidc, bridge := c.Static.Auth.Gateway, c.Static.Storage, c.App.Auth.OidcClient, c.App.MediaWiki.Bridge
	var resolved Secrets
	// 两个对称密钥在此从 base64 解码并校验长度。
	for _, item := range []struct {
		field, inline, envName, defaultEnv string
		destination                        *[]byte
	}{
		{"auth.gateway.cookieSecret", g.CookieSecret, g.CookieSecretEnv, defaultCookieSecretEnv, &resolved.Cookie},
		{"auth.gateway.storageEncryptionKey", g.StorageKey, g.StorageKeyEnv, defaultStorageKeyEnv, &resolved.Storage},
	} {
		value, err := resolveSecret(item.field, item.inline, item.envName, item.defaultEnv, true)
		if err != nil {
			return err
		}
		key, err := security.Key(value)
		if err != nil {
			return &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: item.field + ": invalid encryption key", Op: "config.secret", Field: item.field, Cause: err}
		}
		*item.destination = key
	}

	// 内联 JWKS 与 Env 来源互斥；内联对象序列化后交给统一解析。
	inlineJWKS := ""
	if g.SigningJWKS != nil {
		if g.SigningEnv != "" {
			return fmt.Errorf("auth.gateway.signingJwks: direct value and Env field cannot both be set")
		}
		data, err := json.Marshal(g.SigningJWKS)
		if err != nil {
			return &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: "auth.gateway.signingJwks: invalid JWKS object", Op: "config.signing", Field: "auth.gateway.signingJwks", Cause: err}
		}
		inlineJWKS = string(data)
	}
	var err error
	resolved.Signing, err = resolveSecret("auth.gateway.signingJwks", inlineJWKS, g.SigningEnv, defaultSigningEnv, true)
	if err != nil {
		return err
	}
	if _, err = security.SigningKey(resolved.Signing); err != nil {
		return &failure.Error{Code: "CONFIG_SECRET_INVALID", Message: "auth.gateway.signingJwks: invalid signing key", Op: "config.signing", Field: "auth.gateway.signingJwks", Cause: err}
	}
	for _, item := range []struct {
		field, inline, envName, defaultEnv string
		destination                        *string
	}{
		{"storage.password", storage.Password, storage.PasswordEnv, defaultDBPasswordEnv, &resolved.DBPassword},
		{"auth.oidc_client.clientSecret", oidc.Secret, oidc.SecretEnv, "", &resolved.OidcClientSecret},
		{"mediawiki.serviceBridge.secret", bridge.Secret, bridge.SecretEnv, "", &resolved.BridgeSecret},
	} {
		*item.destination, err = resolveSecret(item.field, item.inline, item.envName, item.defaultEnv, true)
		if err != nil {
			return err
		}
	}
	// 服务桥接密钥至少 32 字节，满足 HMAC 强度要求。
	if len(resolved.BridgeSecret) < 32 {
		return fmt.Errorf("mediawiki.serviceBridge.secret: requires at least 32 bytes")
	}

	resolved.ClientSecrets = map[string]string{}
	seen := map[string]bool{}
	for i, client := range g.Registration.Clients {
		field := fmt.Sprintf("auth.gateway.registration.preRegisteredClients[%d]", i)
		if client.ID == "" || seen[client.ID] {
			return fmt.Errorf("%s.client_id: must be nonempty and unique", field)
		}
		seen[client.ID] = true
		// 仅 confidential 客户端方法要求密钥，public/none 允许留空。
		required := false
		switch client.AuthMethod {
		case "", "none":
		case "client_secret_basic", "client_secret_post":
			required = true
		default:
			return fmt.Errorf("%s.token_endpoint_auth_method: unsupported method", field)
		}
		value, err := resolveSecret(field+".clientSecret", client.Secret, client.SecretEnv, "", required)
		if err != nil {
			return err
		}
		if value != "" {
			resolved.ClientSecrets[client.ID] = value
		}
	}
	// Failed candidates never leave a partially resolved credential set behind.
	c.Secrets = resolved
	return nil
}
