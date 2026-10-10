package config

import (
	"errors"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
	"net"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
)

func LoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()
}

func ValidURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		(u.Scheme == "https" || (u.Scheme == "http" && LoopbackURL(raw)))
}

func ValidAPIURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		(u.Scheme == "https" || u.Scheme == "http")
}

func (c *Config) Validate() (err error) {
	// Validation explanations below are literals or server-generated field names.
	// Keep them useful to CLI operators while preserving their cause internally.
	defer func() {
		if err != nil && failure.Known(err) == nil {
			err = &failure.Error{Code: "CONFIG_INVALID", Message: err.Error(), Op: "config.validate", Field: "configuration", Cause: err}
		}
	}()
	if err := c.validateSite(); err != nil {
		return err
	}
	s, g, k, m := c.Static.Server, c.Static.Auth.Gateway, c.App.Auth.OidcClient, c.App.MediaWiki
	// 静态与动态文件必须声明支持的版本。
	if c.Static.Version != 1 || c.App.Version != 1 {
		return errors.New("unsupported configVersion")
	}

	// 对外 URL 统一要求 HTTPS，仅回环地址允许 HTTP。
	for _, v := range []string{s.PublicURL, g.Issuer, g.Resource, k.Issuer, k.RedirectURI} {
		if !ValidURL(v) {
			return errors.New("URLs require HTTPS, or HTTP on loopback, without userinfo/query/fragment")
		}
	}
	if !ValidAPIURL(m.APIURL) {
		return errors.New("mediawiki.apiUrl requires HTTP or HTTPS without userinfo/query/fragment")
	}
	if err := c.validateWikiOrigins(); err != nil {
		return err
	}
	// issuer、resource 与回调地址必须由 publicUrl 派生，保证 OAuth 元数据自洽。
	if g.Issuer != strings.TrimRight(s.PublicURL, "/") || g.Resource != g.Issuer+"/mcp" || k.RedirectURI != g.Issuer+"/auth/oidc_client/callback" {
		return errors.New("issuer/resource/OIDC callback must match publicUrl")
	}
	// issuer 必须位于根路径，避免父子路径歧义。
	u, _ := url.Parse(g.Issuer)
	if u.Path != "" {
		return errors.New("issuer must use the root path without trailing slash")
	}

	// 监听端口范围与请求体下限、Host 白名单均不可为空。
	if s.Port < 1 || s.Port > 65535 || s.RequestBodyMaxBytes < 1024 || len(s.AllowedHosts) == 0 {
		return errors.New("invalid HTTP listener or limits")
	}
	// 仅支持不透明访问令牌；业务 scope 已在代码中固定，不再由配置声明。
	if g.AccessTokenFormat != "opaque" {
		return errors.New("opaque access tokens are required")
	}
	// 同意页默认勾选的 scope 必须是已知业务范围。
	for _, scope := range c.App.Auth.Consent.DefaultSelectedScopes {
		if !slices.Contains(domain.BusinessScopes(), scope) {
			return errors.New("unsupported default selected scope")
		}
	}

	// 各类 TTL、容量与超时一律必须为正数。
	for _, n := range []int{g.CodeTTL, g.AccessTTL, g.InteractionTTL, g.RefreshTTL, g.GrantTTL, g.Registration.MaxClients, g.Registration.IdleTTL, g.Fetch.TimeoutMS, m.TimeoutMS, m.PageCacheTTL, c.Static.Storage.PoolSize} {
		if n <= 0 {
			return errors.New("TTL, limits, timeout and pool size must be positive")
		}
	}
	// 访问令牌有效期不得超过授权上限；元数据抓取限制需合理。
	if g.AccessTTL > g.GrantTTL || g.Fetch.MaxBytes <= 0 || g.Fetch.MaxRedirects < 0 {
		return errors.New("invalid authorization or metadata limits")
	}

	// 目前仅支持 MySQL，且库名、用户名与端口必填。
	if c.Static.Storage.Driver != "mysql" || c.Static.Storage.Database == "" || c.Static.Storage.Username == "" || c.Static.Storage.Port < 1 || c.Static.Storage.Port > 65535 {
		return errors.New("MySQL database, username and valid port are required")
	}
	// OIDC 需含 openid scope；MediaWiki 站点与桥接 keyId 必填。
	if k.ClientID == "" || !slices.Contains(k.Scopes, "openid") || m.SiteID == "" || m.Bridge.KeyID == "" {
		return errors.New("OIDC and Bridge identity configuration is incomplete")
	}
	// keyId 进入协议头部，禁止引号、反斜杠与换行等注入字符。
	if strings.ContainsAny(m.Bridge.KeyID, "\"\\\r\n") || len(m.Bridge.KeyID) > 128 {
		return errors.New("invalid Bridge keyId")
	}

	// 日志级别限定为已知的四种取值。
	if !slices.Contains([]string{"trace", "debug", "info", "warn", "error"}, c.App.Logging.Level) {
		return errors.New("invalid logging level")
	}
	if !slices.Contains([]string{"text", "json"}, c.App.Logging.ConsoleFormat) {
		return errors.New("invalid logging consoleFormat")
	}
	if c.App.Logging.File.Enabled && (strings.TrimSpace(c.App.Logging.File.Path) == "" || (c.App.Logging.File.Split && strings.TrimSpace(c.App.Logging.File.ErrorPath) == "")) {
		return errors.New("logging file paths are required")
	}
	if err := c.resolveSecrets(); err != nil {
		return err
	}
	if err := c.validateTLS(); err != nil {
		return err
	}

	// 编辑会话与结果输出的大小、并发和操作数上限必须相互匹配。
	if c.App.Editing.TTL < 1 || c.App.Editing.TTL > 3600 || c.App.Editing.MaxActive < 1 || c.App.Editing.MaxOperations < 1 || c.App.Editing.MaxOperations > 64 || c.App.Editing.MaxContent < 1 || c.App.Results.TextMax < 1 || c.App.Results.TextMax > c.App.Editing.MaxContent || c.App.Results.PreviewMax < 1 {
		return errors.New("invalid editing or result limits")
	}
	ucfg := c.App.Uploads
	// 保留时长需覆盖任务与暂存 TTL，预留额度需逐级不小于单文件上限。
	if ucfg.TaskTTL < 1 || ucfg.StagedTTL < 1 || ucfg.Retention < ucfg.TaskTTL || ucfg.Retention < ucfg.StagedTTL || ucfg.MaxBytes < 1 || ucfg.MaxActive < 1 || ucfg.ReservedPrincipal < ucfg.MaxBytes || ucfg.ReservedGlobal < ucfg.ReservedPrincipal || ucfg.TempPath == "" || len(ucfg.Mimes) == 0 {
		return errors.New("invalid upload limits")
	}

	policy := c.App.Additions.Foreign
	// 外部页面确认策略固定使用 has_edited 豁免语义。
	if policy.Exemption != "has_edited" || policy.TTL < 1 || policy.Version < 1 || len(policy.Operations) == 0 {
		return errors.New("invalid contribution confirmation policy")
	}
	// 操作类型须在白名单内且不可重复。
	seen := map[string]bool{}
	for _, op := range policy.Operations {
		if !slices.Contains([]string{"commit", "move", "delete", "uploadOverwrite"}, op) || seen[op] {
			return errors.New("invalid or duplicate confirmation operation")
		}
		seen[op] = true
	}
	// AI 修订标签映射必须覆盖三个内部名称，且值不含分隔符或换行。
	for _, key := range []string{"ai_organized", "ai_assisted", "ai_generated"} {
		if c.App.Additions.Tags.Mapping[key] == "" || strings.ContainsAny(c.App.Additions.Tags.Mapping[key], "|\r\n") {
			return errors.New("AI mapping requires the three internal tag names")
		}
	}
	if len(c.App.Additions.Tags.Mapping) != 3 {
		return errors.New("unsupported AI mapping key")
	}

	// 校验通过后把相对路径锚定到配置目录，供后续运行阶段直接使用。
	if c.Static.Storage.TLS.CAPath != "" && !filepath.IsAbs(c.Static.Storage.TLS.CAPath) {
		c.Static.Storage.TLS.CAPath = filepath.Join(c.Dir, c.Static.Storage.TLS.CAPath)
	}
	if c.App.Uploads.TempPath != "" && !filepath.IsAbs(c.App.Uploads.TempPath) {
		c.App.Uploads.TempPath = filepath.Join(c.Dir, c.App.Uploads.TempPath)
	}
	return nil
}
