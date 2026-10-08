package resource

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

type principalKey struct{}

func PrincipalFromContext(ctx context.Context) (*application.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(*application.Principal)
	return p, ok
}

func (s *Service) Bearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析 Bearer 头，格式不符时返回带资源元数据的质询
		parts := strings.Fields(r.Header.Get("Authorization"))
		challenge := `Bearer resource_metadata="` + s.Config.Static.Auth.Gateway.Issuer + `/.well-known/oauth-protected-resource/mcp", scope="mcp:access"`
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			w.Header().Set("WWW-Authenticate", challenge)
			writeJSON(w, r, 401, map[string]string{"error": "authentication_required"})
			return
		}
		p, err := s.ValidateToken(r.Context(), parts[1])
		if err != nil {
			failure.Log(r.Context(), "auth.bearer", err)
			// 令牌本身无效或缺失才返回质询，基础设施故障不应伪装成 401
			if !errors.Is(err, application.ErrInvalidToken) && !errors.Is(err, application.ErrAuthenticationRequired) {
				status := failure.HTTPStatus(err)
				// 上游依赖超时对客户端表现为服务不可用
				// Bearer dependency timeouts represent service unavailability.
				if status == 502 || status == 504 {
					status = 503
				}
				writeJSON(w, r, status, map[string]string{"error": "server_error"})
				return
			}
			w.Header().Set("WWW-Authenticate", challenge+`, error="invalid_token"`)
			writeJSON(w, r, 401, map[string]string{"error": "invalid_token"})
			return
		}

		// 令牌有效但缺少 mcp:access 时返回权限不足
		if !slices.Contains(p.Scopes, "mcp:access") {
			w.Header().Set("WWW-Authenticate", challenge+`, error="insufficient_scope"`)
			writeJSON(w, r, 403, map[string]string{"error": "insufficient_scope"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}
