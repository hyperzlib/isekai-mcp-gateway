package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"isekai-mcp-gateway/internal/security"
)

// InitDevelopment writes local configuration once. Existing files are never
// overwritten so rerunning the command cannot discard developer credentials.
func InitDevelopment(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	c := Defaults()
	if err := Decode(filepath.Join(dir, "server.example.json"), &c.Static); err != nil {
		return err
	}
	if err := Decode(filepath.Join(dir, "app.example.json"), &c.App); err != nil {
		return err
	}
	c.Static.Server.PublicURL = "http://127.0.0.1:3000"
	c.Static.Server.AllowedHosts = []string{"127.0.0.1:3000", "localhost:3000"}
	c.Static.Auth.Gateway.Issuer = c.Static.Server.PublicURL
	c.Static.Auth.Gateway.Resource = c.Static.Server.PublicURL + "/mcp"
	c.Static.Auth.Gateway.Registration.Dynamic = true
	c.App.Auth.OidcClient.Issuer = "http://127.0.0.1:8080/realms/isekai"
	c.App.Auth.OidcClient.RedirectURI = c.Static.Server.PublicURL + "/auth/oidc_client/callback"
	c.App.MediaWiki.APIURL = "http://127.0.0.1/api.php"
	keys, err := security.GenerateKeys()
	if err != nil {
		return err
	}
	keys.SigningJWKS.Keys[0].KeyID = "development-v1"
	jwks, err := json.Marshal(keys.SigningJWKS)
	if err != nil {
		return err
	}
	cookie, store := keys.CookieSecret, keys.StorageEncryptionKey
	// PowerShell 单引号字符串以双写单引号转义，避免密钥内容破坏生成的脚本。
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	lines := []string{
		"# Local development secrets. Fill placeholders, then dot-source this file.",
		"$env:GATEWAY_OAUTH_COOKIE_SECRET = " + quote(cookie),
		"$env:GATEWAY_OAUTH_STORAGE_KEY = " + quote(store),
		"$env:GATEWAY_OAUTH_SIGNING_JWKS = " + quote(string(jwks)),
		"$env:GATEWAY_DB_PASSWORD = 'FILL_MYSQL_PASSWORD'",
		"$env:GATEWAY_OIDC_CLIENT_SECRET = 'FILL_OIDC_CLIENT_SECRET'",
		"$env:ISEKAI_SERVICE_BRIDGE_SECRET = 'FILL_EXISTING_BRIDGE_SECRET_AT_LEAST_32_BYTES'",
	}
	// 任一目标文件已存在即拒绝，避免重跑命令覆盖开发者凭据。
	for name := range map[string]bool{"server.json": true, "app.json": true, "development.env.ps1": true} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("development config already exists; refusing to overwrite")
		}
	}
	for name, value := range map[string]any{"server.json": c.Static, "app.json": c.App} {
		b, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, name), append(b, '\n'), 0600); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "development.env.ps1"), []byte(strings.Join(lines, "\r\n")+"\r\n"), 0600)
}
