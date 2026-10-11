package zitadel

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/zitadel/oidc/v3/pkg/oidc"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
)

func (s *Service) TokenEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_request"})
		return
	}
	// 仅支持授权码与刷新令牌两种授权类型
	grant := r.Form.Get("grant_type")
	if grant != "authorization_code" && grant != "refresh_token" {
		writeJSON(w, r, 400, map[string]string{"error": "unsupported_grant_type"})
		return
	}

	// OAuth 参数必须唯一，出现重复即视为非法请求
	for _, key := range []string{"resource", "grant_type", "code", "refresh_token", "client_id", "redirect_uri", "code_verifier", "scope"} {
		if len(r.Form[key]) > 1 {
			writeJSON(w, r, 400, map[string]string{"error": "invalid_request"})
			return
		}
	}

	// resource 必须指向本网关的 MCP 端点
	resource := r.Form.Get("resource")
	if resource != s.Config.Static.Auth.Gateway.Resource {
		writeJSON(w, r, 400, map[string]string{"error": "invalid_target"})
		return
	}
	id := r.Form.Get("client_id")
	// Basic 认证携带的 client_id 需先解码，且不得与表单参数冲突
	if basic, _, ok := r.BasicAuth(); ok {
		decoded, e := url.QueryUnescape(basic)
		if e != nil || id != "" && id != decoded {
			writeJSON(w, r, 401, map[string]string{"error": "invalid_client"})
			return
		}
		id = decoded
	}
	client, err := s.GetClientByClientID(r.Context(), id)
	if err != nil {
		if errors.Is(err, oidc.ErrUnauthorizedClient()) {
			writeJSON(w, r, 400, map[string]string{"error": "unauthorized_client", "error_description": domain.BlockedClientDescription})
		} else if errors.Is(err, oidc.ErrServerError()) {
			writeJSON(w, r, 500, map[string]string{"error": "server_error"})
		} else {
			writeJSON(w, r, 401, map[string]string{"error": "invalid_client"})
		}
		return
	}
	// 客户端实际使用的认证方式必须与注册方式一致，避免认证降级
	_, _, basic := r.BasicAuth()
	if client.AuthMethod() == oidc.AuthMethodBasic && !basic || client.AuthMethod() == oidc.AuthMethodPost && basic || client.AuthMethod() == oidc.AuthMethodNone && (basic || r.Form.Get("client_secret") != "") {
		writeJSON(w, r, 401, map[string]string{"error": "invalid_client"})
		return
	}
	info := application.RequestInfo{Resource: resource, ClientID: id, Code: r.Form.Get("code")}
	// 刷新令牌请求保留 scope 信息，供网关合并后再校验
	if grant == "refresh_token" {
		_, info.RefreshScopesPresent = r.Form["scope"]
		info.RefreshScopes = strings.Fields(r.Form.Get("scope"))
		// Gateway merges basic scopes with inherited custom scopes before provider validation.
		r.Form.Del("scope")
	}
	ctx := context.WithValue(r.Context(), application.RequestKey{}, info)
	s.providerResponse(w, r.WithContext(ctx), true)
}
