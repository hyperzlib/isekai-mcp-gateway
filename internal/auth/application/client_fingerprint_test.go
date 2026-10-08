package application

import (
	"encoding/json"
	"isekai-mcp-gateway/internal/auth/domain"
	"testing"
)

func TestClientFingerprintV2(t *testing.T) {
	s := new(Service)
	base := func() Client {
		return Client{Name: "OpenCode", URI: "https://opencode.ai", Redirects: []string{"http://127.0.0.1:19876/callback"}, Grants: []string{"authorization_code", "refresh_token"}, Scope: "wiki:create offline_access"}
	}
	c := base()
	if err := s.validateClient(&c, true); err != nil {
		t.Fatal(err)
	}
	fp := c.Fingerprint
	cases := []struct {
		name   string
		change func(*Client)
		same   bool
	}{
		{"port", func(c *Client) { c.Redirects = []string{"http://127.0.0.1:43210/callback"} }, true},
		{"no_port", func(c *Client) { c.Redirects = []string{"http://127.0.0.1/callback"} }, true},
		{"duplicates", func(c *Client) {
			c.Redirects = append(c.Redirects, "http://127.0.0.1:43210/callback")
			c.Grants = []string{"refresh_token", "authorization_code", "refresh_token"}
			c.Responses = []string{"code", "code"}
			c.Scope = "offline_access wiki:read wiki:edit wiki:create mcp:access"
		}, true},
		{"name", func(c *Client) { c.Name = "Another" }, false},
		{"uri", func(c *Client) { c.URI = "https://another.example" }, false},
		{"missing_name", func(c *Client) { c.Name = "" }, false},
		{"missing_uri", func(c *Client) { c.URI = "" }, false},
		{"path", func(c *Client) { c.Redirects = []string{"http://127.0.0.1:19876/other"} }, false},
		{"escaped_path", func(c *Client) { c.Redirects = []string{"http://127.0.0.1:19876/%63allback"} }, false},
		{"query", func(c *Client) { c.Redirects = []string{"http://127.0.0.1:19876/callback?q=x"} }, false},
		{"ipv6", func(c *Client) { c.Redirects = []string{"http://[::1]:19876/callback"} }, false},
		{"logo_trust", func(c *Client) {
			logo := "https://images.example/a.png"
			c.LogoURL = &logo
			c.TrustLevel = domain.TrustTrusted
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.change(&c)
			if err := s.validateClient(&c, true); err != nil {
				t.Fatal(err)
			}
			if (c.Fingerprint == fp) != tc.same {
				t.Fatalf("fingerprint equality = %v", c.Fingerprint == fp)
			}
		})
	}
	for _, prefix := range []string{"https://127.0.0.1:", "http://localhost:", "https://example.com:"} {
		a, b := base(), base()
		a.Redirects = []string{prefix + "1234/callback"}
		b.Redirects = []string{prefix + "5678/callback"}
		if err := s.validateClient(&a, true); err != nil {
			t.Fatal(err)
		}
		if err := s.validateClient(&b, true); err != nil {
			t.Fatal(err)
		}
		if a.Fingerprint == b.Fingerprint {
			t.Fatal("unexpected port normalization", prefix)
		}
	}
	a, b := base(), base()
	a.Redirects = []string{"http://[::1]:1234/callback"}
	b.Redirects = []string{"http://[::1]:5678/callback"}
	s.validateClient(&a, true)
	s.validateClient(&b, true)
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("IPv6 dynamic port")
	}
	var minimal Client
	json.Unmarshal([]byte(`{"redirect_uris":["http://127.0.0.1:1234/callback"]}`), &minimal)
	if err := s.validateClient(&minimal, true); err != nil {
		t.Fatal(err)
	}
	if minimal.Fingerprint == "" {
		t.Fatal("optional metadata required")
	}
	for _, raw := range []string{"http://images.example/a.png", "https://user@images.example/a", "https://images.example/a#x", "https://images.example/a; img-src *", "data:image/png;base64,x"} {
		c := base()
		c.LogoURL = &raw
		if s.validateClient(&c, true) == nil {
			t.Fatal("invalid logo accepted", raw)
		}
	}
}
