package zitadel

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"isekai-mcp-gateway/internal/auth/application"
)

func (c *Client) GetID() string { return c.ID }

func (c *Client) RedirectURIs() []string { return c.Redirects }

func (c *Client) PostLogoutRedirectURIs() []string { return nil }

func (c *Client) ApplicationType() op.ApplicationType {
	if c.TokenAuthMethod == "none" {
		return op.ApplicationTypeNative
	}
	return op.ApplicationTypeWeb
}

func (c *Client) AuthMethod() oidc.AuthMethod { return oidc.AuthMethod(c.TokenAuthMethod) }

func (c *Client) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}

func (c *Client) GrantTypes() []oidc.GrantType {
	g := make([]oidc.GrantType, len(c.Grants))
	for i, v := range c.Grants {
		g[i] = oidc.GrantType(v)
	}
	return g
}

func (c *Client) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeBearer }

func (c *Client) IDTokenLifetime() time.Duration { return 5 * time.Minute }

func (c *Client) DevMode() bool { return false }

func (c *Client) IDTokenUserinfoClaimsAssertion() bool { return false }

func (c *Client) ClockSkew() time.Duration { return 0 }

func (c *Client) IsScopeAllowed(scope string) bool {
	return slices.Contains(strings.Fields(c.Scope), scope) || slices.Contains(c.CustomScopes, scope)
}

func (c *Client) RestrictAdditionalIdTokenScopes() func([]string) []string {
	return func(s []string) []string { return s }
}

func (c *Client) RestrictAdditionalAccessTokenScopes() func([]string) []string {
	return func(s []string) []string { return s }
}

func (c *Client) LoginURL(id string) string { return "/auth/interaction/" + url.PathEscape(id) }

type Client struct {
	*application.Client
	CustomScopes []string
}
