package oidcclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

type OidcConnect struct {
	provider *oidc.Provider
	oauth    oauth2.Config
	http     *http.Client
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
	}}, nil
}

func (c *OidcConnect) AuthorizationURL(state, nonce, verifier string) string {
	// 使用 S256 PKCE 与 nonce 构造授权请求
	return c.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
}

// Authenticate returns only a verified subject; raw tokens stay inside the integration.
func (c *OidcConnect) Authenticate(ctx context.Context, code, verifier, nonce string) (string, error) {
	ctx = oidc.ClientContext(ctx, c.http)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.http)

	// 用授权码与 PKCE verifier 换取 token
	token, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		var response *oauth2.RetrieveError

		// 授权码失效需用户重新登录
		if errors.As(err, &response) && response.ErrorCode == "invalid_grant" {
			return "", failure.New("LOGIN_EXCHANGE_FAILED", "登录换码失败，请重新登录。", "oidc.exchange", err)
		}

		// 429 与 5xx 视为可重试的上游错误
		if errors.As(err, &response) && response.Response != nil {
			status := response.Response.StatusCode
			return "", &failure.Error{Code: failure.Upstream, Message: "身份服务换码失败。", Op: "oidc.exchange", Retryable: status == 429 || status >= 500, Cause: err, UpstreamStatus: status}
		}
		return "", failure.Dependency("oidc.exchange", err)
	}

	// 要求上游返回 ID token
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return "", failure.New(failure.Upstream, "身份服务没有返回 ID token。", "oidc.exchange", nil)
	}

	// 校验 ID token 的签名与声明
	id, err := c.provider.Verifier(&oidc.Config{ClientID: c.oauth.ClientID}).Verify(ctx, raw)
	if err != nil {
		// 上下文取消/超时与网络错误归类为依赖不可用
		if ctx.Err() != nil {
			return "", failure.Dependency("oidc.verify", ctx.Err())
		}
		var network net.Error
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) {
			return "", failure.Dependency("oidc.verify", err)
		}
		return "", failure.New("LOGIN_VERIFICATION_FAILED", "身份验证失败。", "oidc.verify", err)
	}

	// nonce 不匹配或缺少 subject 视为验证失败，防重放
	if id.Nonce != nonce || id.Subject == "" {
		return "", failure.New("LOGIN_VERIFICATION_FAILED", "身份验证失败。", "oidc.verify", nil)
	}
	return id.Subject, nil
}
