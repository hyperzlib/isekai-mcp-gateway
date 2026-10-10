package wiki

import (
	"context"
	"net/url"
)

// pageURL only exposes URLs for pages with a public page ID.
func (s *Service) pageURL(ctx context.Context, c *Call, p PageRef) (string, error) {
	if p.PageID > 0 {
		p.Title = ""
	}
	params, err := pageParams(p)
	if err != nil {
		return "", err
	}
	q := url.Values(params)
	q.Set("action", "query")
	q.Set("prop", "info")
	q.Set("inprop", "url")
	v, err := c.Client.Query(ctx, c.Credentials, q)
	if err != nil {
		return "", err
	}
	a, err := pages(v)
	if err != nil {
		return "", err
	}
	pg := object(a[0])
	if _, invalid := pg["invalid"]; invalid {
		return "", errCode("UPSTREAM_RESPONSE_INVALID", "Wiki rejected the saved page title.")
	}
	if _, missing := pg["missing"]; missing || number(pg["pageid"]) <= 0 {
		return "", nil
	}
	u := str(pg["fullurl"])
	if u == "" {
		return "", errCode("UPSTREAM_RESPONSE_INVALID", "Wiki omitted the page URL.")
	}
	return u, nil
}
