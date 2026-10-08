package zitadel

import (
	"context"
	"errors"
	"slices"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/failure"
)

func (s *Service) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}

func (s *Service) SetUserinfoFromRequest(ctx context.Context, u *oidc.UserInfo, req op.IDTokenRequest, scopes []string) error {
	var a application.Account
	row, err := s.Store.Get(s.Store.Database().WithContext(ctx), "accounts", req.GetSubject(), &a, false)
	// 账号不存在映射为无效授权，基础设施错误交由统一错误映射
	if err != nil {
		if errors.Is(err, application.ErrRecordNotFound) {
			return oidc.ErrInvalidGrant()
		}
		return providerError(ctx, err)
	}

	// 仅 active 账号可返回 userinfo
	if row.State != "active" {
		return oidc.ErrInvalidGrant()
	}
	u.Subject = a.ID
	// 仅当请求包含 profile scope 时才暴露显示名与用户名
	if slices.Contains(scopes, "profile") {
		u.Name, u.PreferredUsername = a.DisplayName, a.Username
	}
	return nil
}

func (s *Service) SetUserinfoFromToken(ctx context.Context, u *oidc.UserInfo, id, subject, origin string) error {
	t, _, _, err := s.AccessByID(s.Store.Database().WithContext(ctx), id, subject)
	if err != nil {
		return providerError(ctx, err)
	}
	return s.SetUserinfoFromRequest(ctx, u, t, t.Scopes)
}

func (s *Service) SetIntrospectionFromToken(ctx context.Context, out *oidc.IntrospectionResponse, id, subject, clientID string) error {
	// 只有已注册的资源服务器客户端才可执行自省，其他情况统一拒绝
	resourceClient, _, err := s.LoadClient(s.Store.Database().WithContext(ctx), clientID, false)
	if err != nil {
		if errors.Is(err, application.ErrRecordNotFound) || !failure.IsInfrastructure(err) {
			return oidc.ErrAccessDenied()
		}
		return providerError(ctx, err)
	}

	// 公共客户端或非资源客户端不具备自省权限
	if !resourceClient.ResourceClient || resourceClient.TokenAuthMethod == "none" {
		return oidc.ErrAccessDenied()
	}
	t, _, row, err := s.AccessByID(s.Store.Database().WithContext(ctx), id, subject)
	if err != nil {
		return providerError(ctx, err)
	}
	out.Active, out.ClientID, out.Subject, out.Issuer, out.TokenType = true, t.ClientID, t.Subject, s.Config.Static.Auth.Gateway.Issuer, "Bearer"
	out.Scope, out.Audience = t.Scopes, []string{t.Resource}
	out.Expiration = oidc.FromTime(row.ExpiresAt)
	return nil
}
