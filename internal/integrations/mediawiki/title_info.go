package mediawiki

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

type TitleInfo struct {
	Title  string `json:"title"`
	PageID int64  `json:"page_id"`
	Exists bool   `json:"exists"`
}

func (c *Client) titleInfoURL(raw string) (*url.URL, error) {
	if c.Config.PublicURL == "" {
		return nil, failure.New("FEATURE_UNAVAILABLE", "Configure mediawiki.publicUrl to resolve page URLs.", "wiki.url", nil)
	}
	target, err := config.ParseWikiOrigin(c.Config.PublicURL)
	if err != nil {
		return nil, failure.New("CONFIG_INVALID", "Invalid mediawiki.publicUrl.", "wiki.url", err)
	}
	invalid := func() (*url.URL, error) {
		return nil, failure.New("INVALID_REQUEST", "Supply an allowed HTTP(S) page URL or a site absolute path.", "wiki.url", nil)
	}
	if raw == "" || strings.ContainsAny(raw, "\\\r\n\x00") {
		return invalid()
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || strings.Contains(u.Path, "\\") {
		return invalid()
	}
	if strings.HasPrefix(raw, "/") {
		if strings.HasPrefix(raw, "//") || u.Host != "" || u.Scheme != "" {
			return invalid()
		}
	} else {
		origin, err := config.HTTPOrigin(u)
		if err != nil {
			return invalid()
		}
		baseOrigin, _ := config.HTTPOrigin(target)
		allowed := origin == baseOrigin
		for _, candidate := range c.Config.PublicOrigins {
			v, err := config.ParseWikiOrigin(candidate)
			if err != nil {
				return nil, failure.New("CONFIG_INVALID", "Invalid mediawiki.publicOrigins.", "wiki.url", err)
			}
			o, _ := config.HTTPOrigin(v)
			allowed = allowed || origin == o
		}
		if !allowed {
			return invalid()
		}
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return invalid()
	}
	q.Set("action", "isekai-title-info")
	q.Del("veaction")
	u.Scheme, u.Host = target.Scheme, target.Host
	u.Fragment, u.RawFragment, u.RawQuery, u.ForceQuery = "", "", q.Encode(), false
	return u, nil
}

// TitleInfo uses the public Action entry point; Bridge credentials only work on api.php.
func (c *Client) TitleInfo(ctx context.Context, raw string) (*TitleInfo, error) {
	u, err := c.titleInfoURL(raw)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, failure.New("INVALID_REQUEST", "Invalid page URL.", "wiki.url", err)
	}
	req.Header.Set("Accept", "application/json")
	h := *c.HTTP
	h.Jar = nil
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := h.Do(req)
	if err != nil {
		return nil, failure.New("API_NETWORK_ERROR", "Wiki URL request failed.", "wiki.url.request", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, &failure.Error{Code: "UPSTREAM_ERROR", Message: "Wiki URL request did not return HTTP 200.", Op: "wiki.url.response", UpstreamStatus: res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		return nil, failure.New("API_NETWORK_ERROR", "Wiki URL response was interrupted.", "wiki.url.read", err)
	}
	if len(data) > 65536 {
		return nil, failure.New("UPSTREAM_RESPONSE_TOO_LARGE", "Wiki title information exceeds 64 KiB.", "wiki.url.read", nil)
	}
	var v struct {
		Title  *string `json:"title"`
		PageID *int64  `json:"page_id"`
		Exists *bool   `json:"exists"`
	}
	if json.Unmarshal(data, &v) != nil || v.Title == nil || strings.TrimSpace(*v.Title) == "" || v.PageID == nil || *v.PageID < 0 || v.Exists == nil || *v.Exists != (*v.PageID > 0) {
		return nil, failure.New("UPSTREAM_RESPONSE_INVALID", "Wiki returned invalid title information.", "wiki.url.decode", nil)
	}
	return &TitleInfo{Title: *v.Title, PageID: *v.PageID, Exists: *v.Exists}, nil
}
