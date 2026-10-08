package zitadel

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/application"
)

func (s *Service) Authorize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_request"})
		return
	}
	client, err := s.GetClientByClientID(r.Context(), r.Form.Get("client_id"))
	if err != nil {
		if errors.Is(err, oidc.ErrServerError()) {
			writeJSON(w, r, 500, map[string]string{"error": "server_error"})
		} else {
			writeJSON(w, r, 400, map[string]string{"error": "invalid_client"})
		}
		return
	}
	// 重定向地址必须精确匹配注册值，并满足 provider 的校验规则
	redirect, state := r.Form.Get("redirect_uri"), r.Form.Get("state")
	if !client.(*Client).ValidRedirect(redirect) {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_redirect_uri"})
		return
	}
	if err = op.ValidateAuthReqRedirectURI(client, redirect, oidc.ResponseTypeCode); err != nil {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_redirect_uri"})
		return
	}

	// 授权参数必须唯一，重复参数按 invalid_request 处理
	for _, key := range []string{"client_id", "redirect_uri", "state", "response_type", "scope", "resource", "code_challenge", "code_challenge_method"} {
		if len(r.Form[key]) > 1 {
			s.AuthError(w, r, redirect, state, "invalid_request", "duplicate authorization parameter")
			return
		}
	}

	// 强制授权码流程并要求 S256 PKCE，challenge 解码后应为 32 字节
	challenge, e := base64.RawURLEncoding.DecodeString(r.Form.Get("code_challenge"))
	if r.Form.Get("response_type") != "code" || r.Form.Get("code_challenge_method") != "S256" || e != nil || len(challenge) != 32 {
		s.AuthError(w, r, redirect, state, "invalid_request", "code and S256 PKCE are required")
		return
	}
	if r.Form.Get("response_mode") != "" && r.Form.Get("response_mode") != "query" {
		s.AuthError(w, r, redirect, state, "invalid_request", "query response mode is required")
		return
	}

	// 不支持静默授权，必须经过交互式同意
	if r.Form.Get("prompt") == "none" {
		s.AuthError(w, r, redirect, state, "consent_required", "interactive consent is required")
		return
	}

	// resource 必须指向本网关，且请求 scope 需在客户端与网关允许范围内
	resource := r.Form.Get("resource")
	if resource != s.Config.Static.Auth.Gateway.Resource {
		s.AuthError(w, r, redirect, state, "invalid_target", "resource must match this MCP endpoint")
		return
	}
	scopes, err := application.NormalizeScopes(strings.Fields(r.Form.Get("scope")), application.BusinessScopes())
	if err != nil || !application.RequestedScopesAllowed(scopes, strings.Fields(client.(*Client).Scope)) {
		s.AuthError(w, r, redirect, state, "invalid_scope", "requested scope is not allowed")
		return
	}

	// offline_access 依赖客户端的刷新令牌授权
	if slices.Contains(scopes, "offline_access") && !slices.Contains(client.(*Client).Grants, "refresh_token") {
		s.AuthError(w, r, redirect, state, "invalid_scope", "refresh grant is unavailable")
		return
	}
	r.Form.Set("scope", strings.Join(scopes, " "))
	// Zitadel's authorization request omits RFC 8707; carry it in context.
	r.Form.Del("resource")
	browser := s.Browser(w, r)
	ctx := context.WithValue(r.Context(), application.RequestKey{}, application.RequestInfo{Browser: browser, Resource: resource, ClientID: client.GetID()})
	s.providerResponse(w, r.WithContext(ctx), false)
}
