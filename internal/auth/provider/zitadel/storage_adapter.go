package zitadel

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/failure"
)

func providerError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	// 刷新令牌失效需保留 provider 的专用错误语义
	if errors.Is(err, domain.ErrInvalidRefreshToken) {
		return op.ErrInvalidRefreshToken
	}
	failure.Record(ctx, err)
	failure.Log(ctx, "oauth.provider", err)
	// 令牌或认证类错误统一降级为 invalid_grant，避免泄露内部细节
	if errors.Is(err, failure.ErrInvalidToken) || errors.Is(err, failure.ErrAuthenticationRequired) {
		return oidc.ErrInvalidGrant()
	}
	// 领域层 OAuth 错误按错误码映射为对应的 OIDC 错误
	var oauthError *domain.OAuthError
	if errors.As(err, &oauthError) {
		var result *oidc.Error
		switch oauthError.Code {
		case "invalid_request":
			result = oidc.ErrInvalidRequest()
		case "invalid_grant":
			result = oidc.ErrInvalidGrant()
		case "invalid_client":
			result = oidc.ErrInvalidClient()
		case "invalid_scope":
			result = oidc.ErrInvalidScope()
		default:
			result = oidc.ErrServerError()
		}
		return result.WithDescription("%s", oauthError.Description)
	}
	return oidc.ErrServerError().WithDescription("%s", "Service operation failed.")
}

func (s *Service) GetClientByClientID(ctx context.Context, id string) (op.Client, error) {
	c, err := s.Service.GetClientByClientID(ctx, id)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	_, custom := domain.SplitScopes(domain.BusinessScopes())
	return &Client{Client: c, CustomScopes: custom}, nil
}
func (s *Service) AuthorizeClientIDSecret(ctx context.Context, id, secret string) error {
	return providerError(ctx, s.Service.AuthorizeClientIDSecret(ctx, id, secret))
}
func (s *Service) CreateAuthRequest(ctx context.Context, request *oidc.AuthRequest, user string) (op.AuthRequest, error) {
	// Preserve the provider's serialized field representation for stored records.
	data, err := json.Marshal(request)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	var input domain.AuthorizationRequest
	if err = json.Unmarshal(data, &input); err != nil {
		return nil, providerError(ctx, err)
	}
	a, err := s.Service.CreateAuthRequest(ctx, &input, user)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	return &authRequest{a}, nil
}
func (s *Service) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	a, err := s.Service.AuthRequestByID(ctx, id)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	return &authRequest{a}, nil
}
func (s *Service) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	a, err := s.Service.AuthRequestByCode(ctx, code)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	return &authRequest{a}, nil
}
func (s *Service) SaveAuthCode(ctx context.Context, id, code string) error {
	return providerError(ctx, s.Service.SaveAuthCode(ctx, id, code))
}
func (s *Service) DeleteAuthRequest(ctx context.Context, id string) error {
	return providerError(ctx, s.Service.DeleteAuthRequest(ctx, id))
}

func unwrapRequest(request op.TokenRequest) domain.TokenRequest {
	if a, ok := request.(*authRequest); ok {
		return a.Interaction
	}
	return request
}
func (s *Service) CreateAccessToken(ctx context.Context, request op.TokenRequest) (string, time.Time, error) {
	id, expiry, err := s.Service.CreateAccessToken(ctx, unwrapRequest(request))
	return id, expiry, providerError(ctx, err)
}
func (s *Service) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, old string) (string, string, time.Time, error) {
	id, next, expiry, err := s.Service.CreateAccessAndRefreshTokens(ctx, unwrapRequest(request), old)
	return id, next, expiry, providerError(ctx, err)
}
func (s *Service) TokenRequestByRefreshToken(ctx context.Context, raw string) (op.RefreshTokenRequest, error) {
	token, err := s.Service.TokenRequestByRefreshToken(ctx, raw)
	if err != nil {
		return nil, providerError(ctx, err)
	}
	return token, nil
}
func (s *Service) GetRefreshTokenInfo(ctx context.Context, client, raw string) (string, string, error) {
	sub, id, err := s.Service.GetRefreshTokenInfo(ctx, client, raw)
	return sub, id, providerError(ctx, err)
}
func (s *Service) RevokeToken(ctx context.Context, token, subject, client string) *oidc.Error {
	err := providerError(ctx, s.Service.RevokeToken(ctx, token, subject, client))
	if err == nil {
		return nil
	}
	var oauthError *oidc.Error
	if errors.As(err, &oauthError) {
		return oauthError
	}
	return oidc.ErrServerError()
}
func (s *Service) TerminateSession(ctx context.Context, subject, client string) error {
	return providerError(ctx, s.Service.TerminateSession(ctx, subject, client))
}
