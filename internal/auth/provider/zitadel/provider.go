package zitadel

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"

	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/outbound"
	"isekai-mcp-gateway/internal/security"
)

type signingKey struct{ jwk jose.JSONWebKey }

func (k signingKey) ID() string { return k.jwk.KeyID }

func (k signingKey) SignatureAlgorithm() jose.SignatureAlgorithm {
	return jose.SignatureAlgorithm(k.jwk.Algorithm)
}

func (k signingKey) Key() any { return k.jwk.Key }

type publicKey struct{ jwk jose.JSONWebKey }

func (k publicKey) ID() string { return k.jwk.KeyID }

func (k publicKey) Algorithm() jose.SignatureAlgorithm {
	return jose.SignatureAlgorithm(k.jwk.Algorithm)
}

func (k publicKey) Use() string { return "sig" }

func (k publicKey) Key() any { return k.jwk.Key }

func New(c *config.Config, store application.RecordStore, bundles *outbound.Manager) (*Service, error) {
	k, err := security.SigningKey(c.Secrets.Signing)
	if err != nil {
		return nil, err
	}
	s := &Service{Service: &application.Service{Config: c, Store: store, Upstream: bundles, FetchMetadata: fetchMetadata}, Signing: signingKey{k}}
	g := c.Static.Auth.Gateway
	// Domain separation avoids sharing an AEAD key between records and tokens.
	key := sha256.Sum256(append(append([]byte{}, c.Secrets.Storage...), []byte("gateway-provider-jwe-v1")...))
	// 各端点统一改写为网关对外暴露的路径
	opts := []op.Option{
		op.WithCrypto(op.NewAES256GCMCrypto(key, "gateway-v1")),
		op.WithCustomAuthEndpoint(op.NewEndpoint("/oauth/authorize")),
		op.WithCustomTokenEndpoint(op.NewEndpoint("/oauth/token")),
		op.WithCustomIntrospectionEndpoint(op.NewEndpoint("/oauth/introspect")),
		op.WithCustomUserinfoEndpoint(op.NewEndpoint("/oauth/userinfo")),
		op.WithCustomRevocationEndpoint(op.NewEndpoint("/oauth/revoke")),
		op.WithCustomKeysEndpoint(op.NewEndpoint("/oauth/jwks")),
	}
	// 本地回环 issuer 允许使用非 HTTPS，方便开发调试
	if config.LoopbackURL(g.Issuer) {
		opts = append(opts, op.WithAllowInsecure())
	}
	// 强制 PKCE S256、仅支持 post 客户端认证与刷新令牌，并合并基础 scope
	provider, err := op.NewProvider(&op.Config{CodeMethodS256: true, AuthMethodPost: true, GrantTypeRefreshToken: true,
		SupportedScopes: append(append([]string{}, application.BusinessScopes()...), application.BasicScopes()...), SupportedClaims: []string{"sub", "name", "preferred_username"},
	}, s, op.StaticIssuer(g.Issuer), opts...)
	if err != nil {
		return nil, err
	}
	s.Provider = provider
	s.Service.Decoder = provider.Crypto()
	return s, nil
}

func (s *Service) SigningKey(context.Context) (op.SigningKey, error) { return s.Signing, nil }

func (s *Service) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{s.Signing.SignatureAlgorithm()}, nil
}

func (s *Service) KeySet(context.Context) ([]op.Key, error) {
	return []op.Key{publicKey{s.Signing.jwk.Public()}}, nil
}

func (s *Service) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, errors.New("private_key_jwt is unsupported")
}

func (s *Service) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, errors.New("JWT profile grant is unsupported")
}

func (s *Service) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return nil, nil
}

// Service adapts application storage and Gateway HTTP extensions to the provider.
type Service struct {
	*application.Service
	Provider  *op.Provider
	Signing   signingKey
	Browser   func(http.ResponseWriter, *http.Request) string
	PageError func(http.ResponseWriter, *http.Request, string, int)
}

var _ op.Storage = (*Service)(nil)
