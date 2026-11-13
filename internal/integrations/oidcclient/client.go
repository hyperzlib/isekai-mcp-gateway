package oidcclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

type OidcConnect struct {
	provider *oidc.Provider
	oauth    oauth2.Config
	http     *http.Client
	// 头像声明配置在建立连接时固化，热重载通过重建 Bundle 生效。
	AvatarClaim   string
	DefaultAvatar string
}

// LoginResult 是一次登录换码后的已验证结果：subject 用于账号关联，
// AvatarURL 是按配置提取的头像地址（可能为空）。
type LoginResult struct {
	Subject   string
	AvatarURL string
}

// AuthorizationOrigin is used to permit the form's upstream redirect in CSP.
func (c *OidcConnect) AuthorizationOrigin() string {
	if !config.ValidURL(c.oauth.Endpoint.AuthURL) {
		return ""
	}

	// 提取 scheme://host，供 CSP 放行上游授权跳转
	u, _ := url.Parse(c.oauth.Endpoint.AuthURL)
	return u.Scheme + "://" + u.Host
}

func New(ctx context.Context, c *config.Config, httpClient *http.Client) (*OidcConnect, error) {
	// 通过 OIDC discovery 加载上游 issuer 元数据
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, httpClient), c.App.Auth.OidcClient.Issuer)
	if err != nil {
		return nil, failure.Dependency("oidc.discovery", err)
	}
	return &OidcConnect{provider: provider, http: httpClient, oauth: oauth2.Config{
		ClientID: c.App.Auth.OidcClient.ClientID, ClientSecret: c.Secrets.OidcClientSecret,
		Endpoint: provider.Endpoint(), RedirectURL: c.App.Auth.OidcClient.RedirectURI, Scopes: c.App.Auth.OidcClient.Scopes,
	}, AvatarClaim: c.App.Auth.OidcClient.AvatarClaim, DefaultAvatar: c.App.Auth.OidcClient.DefaultAvatar}, nil
}

func (c *OidcConnect) AuthorizationURL(state, nonce, verifier string) string {
	// 使用 S256 PKCE 与 nonce 构造授权请求
	return c.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
}

// Authenticate returns only the verified subject and avatar; raw tokens stay
// inside the integration.
func (c *OidcConnect) Authenticate(ctx context.Context, code, verifier, nonce string) (LoginResult, error) {
	ctx = oidc.ClientContext(ctx, c.http)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)

	// 用授权码与 PKCE verifier 换取 token
	token, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		// 记录脱敏后的换码请求与上游响应（密钥已掩码），便于按错误编号排查
		t := failure.UpstreamTrace{
			Method:         "POST",
			URL:            c.oauth.Endpoint.TokenURL,
			RequestHeaders: "Content-Type: application/x-www-form-urlencoded",
			RequestBody: url.Values{
				"client_id":     {c.oauth.ClientID},
				"grant_type":    {"authorization_code"},
				"code":          {code},
				"redirect_uri":  {c.oauth.RedirectURL},
				"client_secret": {"[REDACTED]"},
				"code_verifier": {"[REDACTED]"},
			}.Encode(),
		}
		var response *oauth2.RetrieveError
		if errors.As(err, &response) {
			if response.Response != nil {
				t.Status = response.Response.StatusCode
			}
			t.ResponseBody = failure.ClipResponseBody([]byte(response.Body))
		}
		failure.TraceUpstream(ctx, "oidc.exchange", t, err)

		// 授权码失效需用户重新登录
		if errors.As(err, &response) && response.ErrorCode == "invalid_grant" {
			return LoginResult{}, failure.New("LOGIN_EXCHANGE_FAILED", "登录换码失败，请重新登录。", "oidc.exchange", err)
		}

		// 429 与 5xx 视为可重试的上游错误
		if errors.As(err, &response) && response.Response != nil {
			status := response.Response.StatusCode
			return LoginResult{}, &failure.Error{Code: failure.Upstream, Message: "身份服务换码失败。", Op: "oidc.exchange", Retryable: status == 429 || status >= 500, Cause: err, UpstreamStatus: status}
		}
		return LoginResult{}, failure.Dependency("oidc.exchange", err)
	}

	// 要求上游返回 ID token
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return LoginResult{}, failure.New(failure.Upstream, "身份服务没有返回 ID token。", "oidc.exchange", nil)
	}

	// 校验 ID token 的签名与声明
	id, err := c.provider.Verifier(&oidc.Config{ClientID: c.oauth.ClientID}).Verify(ctx, raw)
	if err != nil {
		// 上下文取消/超时与网络错误归类为依赖不可用
		if ctx.Err() != nil {
			return LoginResult{}, failure.Dependency("oidc.verify", ctx.Err())
		}
		var network net.Error
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) {
			return LoginResult{}, failure.Dependency("oidc.verify", err)
		}
		return LoginResult{}, failure.New("LOGIN_VERIFICATION_FAILED", "身份验证失败。", "oidc.verify", err)
	}

	// nonce 不匹配或缺少 subject 视为验证失败，防重放
	if id.Nonce != nonce || id.Subject == "" {
		return LoginResult{}, failure.New("LOGIN_VERIFICATION_FAILED", "身份验证失败。", "oidc.verify", nil)
	}
	return LoginResult{Subject: id.Subject, AvatarURL: c.avatar(ctx, id, token)}, nil
}

// avatar 按配置的头像声明路径提取头像；id_token. 前缀从已验证 ID token
// claims 取值，userinfo. 前缀调 UserInfo 端点取值。取值无效时回退默认头像。
func (c *OidcConnect) avatar(ctx context.Context, id *oidc.IDToken, token *oauth2.Token) string {
	claim := c.AvatarClaim
	value := ""
	switch {
	case claim == "":
		// 未配置头像声明时不取头像。
	case strings.HasPrefix(claim, "id_token."):
		var claims map[string]any
		if id.Claims(&claims) == nil {
			value = claimPath(claims, strings.TrimPrefix(claim, "id_token."))
		}
	case strings.HasPrefix(claim, "userinfo."):
		if info, err := c.provider.UserInfo(ctx, oauth2.StaticTokenSource(token)); err == nil {
			var claims map[string]any
			if info.Claims(&claims) == nil {
				value = claimPath(claims, strings.TrimPrefix(claim, "userinfo."))
			}
		}
	}
	if !validAvatarURL(value) {
		value = ""
	}
	if value == "" {
		return c.DefaultAvatar
	}
	return value
}

// claimPath 支持点号嵌套的声明路径，逐层进入 map 取值。
func claimPath(claims map[string]any, path string) string {
	var current any = claims
	for _, key := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	s, _ := current.(string)
	return s
}

// validAvatarURL 只接受合法的 https URL 作为头像地址。
func validAvatarURL(raw string) bool {
	if strings.ContainsAny(raw, "\"'\\\r\n ") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
