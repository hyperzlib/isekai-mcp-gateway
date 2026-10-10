package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// HTTPOrigin compares origins with explicit default ports and normalized casing.
func HTTPOrigin(u *url.URL) (string, error) {
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.HasSuffix(u.Host, ":") || strings.ContainsAny(u.Host, "\\ \t\r\n") {
		return "", fmt.Errorf("requires an HTTP(S) origin without userinfo")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if scheme == "https" {
			port = "443"
		}
	} else {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return "", fmt.Errorf("invalid origin port")
		}
		port = strconv.Itoa(p)
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port), nil
}

func ParseWikiOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if _, err := HTTPOrigin(u); err != nil {
		return nil, err
	}
	if (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return nil, fmt.Errorf("origin must not contain a path, query or fragment")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, nil
}

func (c *Config) validateWikiOrigins() error {
	if c.App.MediaWiki.PublicURL != "" {
		if _, err := ParseWikiOrigin(c.App.MediaWiki.PublicURL); err != nil {
			return fmt.Errorf("mediawiki.publicUrl: %w", err)
		}
	}
	for _, raw := range c.App.MediaWiki.PublicOrigins {
		if _, err := ParseWikiOrigin(raw); err != nil {
			return fmt.Errorf("mediawiki.publicOrigins: %w", err)
		}
	}
	return nil
}
