package domain

import (
	"net"
	"net/url"
	"slices"
	"time"
)

type Client struct {
	URI             string     `json:"client_uri,omitempty"`
	LogoURL         *string    `json:"logo_uri,omitempty"`
	TrustLevel      TrustLevel `json:"-"`
	ID              string     `json:"client_id"`
	Name            string     `json:"client_name"`
	Redirects       []string   `json:"redirect_uris"`
	Grants          []string   `json:"grant_types"`
	Responses       []string   `json:"response_types"`
	TokenAuthMethod string     `json:"token_endpoint_auth_method"`
	Scope           string     `json:"scope"`
	SecretHash      string     `json:"-"`
	Source          string     `json:"-"`
	Fingerprint     string     `json:"-"`
	FetchedAt       time.Time  `json:"-"`
	ResourceClient  bool       `json:"-"`
}

// Native loopback clients may select a free port. All other URI components,
// including the loopback host, remain registered values.
func (c *Client) ValidRedirect(raw string) bool {
	if slices.Contains(c.Redirects, raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || c.TokenAuthMethod != "none" || u.Scheme != "http" || !net.ParseIP(u.Hostname()).IsLoopback() {
		return false
	}
	// 回环地址允许端口可变，但其余 URI 组件必须与某个已注册地址完全一致。
	for _, registered := range c.Redirects {
		v, err := url.Parse(registered)
		if err == nil && v.Scheme == u.Scheme && v.Hostname() == u.Hostname() && v.EscapedPath() == u.EscapedPath() && v.RawQuery == u.RawQuery && u.User == nil && u.Fragment == "" {
			return true
		}
	}
	return false
}
