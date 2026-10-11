package domain

import (
	"encoding/json"
	"isekai-mcp-gateway/internal/security"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
)

type TrustLevel string

const (
	TrustBlocked TrustLevel = "blocked"
	TrustUnknown TrustLevel = "unknown"
	TrustKnown   TrustLevel = "known"
	TrustTrusted TrustLevel = "trusted"
)

func DefaultTrust(source string) TrustLevel {
	switch source {
	case "pre":
		return TrustTrusted
	case "cimd":
		return TrustKnown
	default:
		return TrustUnknown
	}
}

func WebMetadataURL(raw string, httpsOnly bool) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 8192 || strings.ContainsAny(raw, "'\"; \\ \t\r\n") {
		return false
	}
	for _, r := range raw {
		if r < 32 || r == 127 {
			return false
		}
	}
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && u.Fragment == "" && !strings.Contains(raw, "#") && (u.Scheme == "https" || (!httpsOnly && u.Scheme == "http"))
}

func ClientFingerprint(c *Client) (string, string, error) {
	scopes, err := NormalizeScopes(strings.Fields(c.Scope), BusinessScopes())
	if err != nil {
		return "", "", err
	}
	redirects := slices.Clone(c.Redirects)
	for i, raw := range redirects {
		u, err := url.Parse(raw)
		if err == nil && c.TokenAuthMethod == "none" && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
			u.Host = net.JoinHostPort(u.Hostname(), "*")
			redirects[i] = u.String()
		}
	}
	sorted := func(a []string) []string { a = slices.Clone(a); slices.Sort(a); return slices.Compact(a) }
	b, err := json.Marshal([]any{"v2", c.Name, c.URI, sorted(redirects), sorted(c.Grants), sorted(c.Responses), sorted(scopes), c.TokenAuthMethod, c.SecretHash, c.ResourceClient})
	if err != nil {
		return "", "", err
	}
	return security.Hash(string(b)), string(b), nil
}

type StoredClient struct {
	Metadata       Client
	SecretHash     string
	Source         string
	Fingerprint    string
	FetchedAt      time.Time
	ResourceClient bool
}

// PrivateApplicationScheme excludes web, script, content and operating-system URI handlers.
func PrivateApplicationScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "", "http", "https", "javascript", "vbscript", "data", "file", "blob", "about", "ftp", "mailto", "tel":
		return false
	default:
		return true
	}
}
