package gatewaymcp

import (
	"context"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/auth/application"
)

// MCP transport contexts can outlive the HTTP request on older protocol
// versions. RequestExtra is the source of the current request's identity.
func (s *Authenticator) CheckMCPRequest(ctx context.Context, extra *mcp.RequestExtra) error {
	// 缺少令牌信息说明请求根本未携带凭据，区别于令牌无效。
	if extra == nil || extra.TokenInfo == nil {
		return application.ErrAuthenticationRequired
	}

	// principal 类型断言、非空、未过期、且具备 mcp:access 权限，任一不满足都判为非法令牌。
	p, ok := extra.TokenInfo.Extra["principal"].(*application.Principal)
	if !ok || p == nil || !p.ExpiresAt.After(time.Now()) || !slices.Contains(p.Scopes, "mcp:access") {
		return application.ErrInvalidToken
	}

	// 最后确认底层 Grant 仍然有效（未被撤销/过期），防止已失效的令牌继续使用。
	_, _, err := s.ActiveGrant(s.Store.Database().WithContext(ctx), p.GrantID, false)
	return err
}

// RequestAuthorizer is the only authentication dependency tools and resources need.
type RequestAuthorizer interface {
	CheckMCPRequest(context.Context, *mcp.RequestExtra) error
}
type Authenticator struct{ *application.Service }

func (s *Authenticator) LogoutMCPRequest(ctx context.Context, extra *mcp.RequestExtra) error {
	// 退出前先校验请求身份，确保只能撤销自己当前的授权。
	if err := s.CheckMCPRequest(ctx, extra); err != nil {
		return err
	}

	p := extra.TokenInfo.Extra["principal"].(*application.Principal)
	return s.Logout(ctx, p)
}

type RequestLogout interface {
	LogoutMCPRequest(context.Context, *mcp.RequestExtra) error
}
