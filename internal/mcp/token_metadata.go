package gatewaymcp

import (
	"context"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"

	"isekai-mcp-gateway/internal/auth/resource"
)

func BearerMetadata(issuer string, next http.Handler) http.Handler {
	// The SDK uses TokenInfo for per-request metadata and transport session
	// identity binding. Include client and Grant to prevent cross-client reuse.
	return sdkauth.RequireBearerToken(func(ctx context.Context, _ string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		p, ok := resource.PrincipalFromContext(ctx)
		if !ok {
			return nil, sdkauth.ErrInvalidToken
		}

		// UserID 拼接客户端与 Grant，作为 SDK 会话身份，避免同一账号下跨客户端复用。
		return &sdkauth.TokenInfo{UserID: p.AccountID + ":" + p.OAuthClientID + ":" + p.GrantID, Scopes: p.Scopes, Expiration: p.ExpiresAt, Extra: map[string]any{"principal": p}}, nil
	}, &sdkauth.RequireBearerTokenOptions{ResourceMetadataURL: issuer + "/.well-known/oauth-protected-resource/mcp", Scopes: []string{"mcp:access"}})(next)
}
