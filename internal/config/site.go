package config

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const DefaultSiteTitle = "Isekai MCP Gateway"

type Site struct {
	Title      string          `json:"title"`
	IconURL    string          `json:"iconUrl"`
	HeaderLink *SiteHeaderLink `json:"headerLink"`
}

type SiteHeaderLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

func validHeaderLinkURL(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\\\"' ") {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && u.Host == "" && u.Scheme == ""
	}
	return u.Scheme == "https" && u.Hostname() != ""
}

func ValidSiteIcon(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "\\'\";#") {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && u.Host == "" && u.Scheme == ""
	}
	return u.Scheme == "https" && u.Hostname() != ""
}

func (c *Config) validateSite() error {
	if strings.TrimSpace(c.App.Site.Title) == "" {
		c.App.Site.Title = DefaultSiteTitle
	}
	if !utf8.ValidString(c.App.Site.Title) || utf8.RuneCountInString(c.App.Site.Title) > 256 {
		return errors.New("site.title must contain at most 256 characters")
	}
	if !ValidSiteIcon(c.App.Site.IconURL) {
		return errors.New("site.iconUrl requires a same-site root path or absolute HTTPS URL without userinfo, fragment or unsafe characters")
	}
	if link := c.App.Site.HeaderLink; link != nil {
		if strings.TrimSpace(link.Label) == "" || !utf8.ValidString(link.Label) {
			return errors.New("site.headerLink.label must not be blank")
		}
		if !validHeaderLinkURL(link.URL) {
			return errors.New("site.headerLink.url requires a same-site root path or absolute HTTPS URL")
		}
	}
	return nil
}
