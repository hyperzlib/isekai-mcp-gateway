package mediawiki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"isekai-mcp-gateway/internal/failure"
)

func (c *Client) Probe(ctx context.Context) error {
	u, err := url.Parse(c.Config.APIURL)
	if err != nil {
		return failure.Wrap("wiki.discovery.prepare", err)
	}
	// 请求 Bridge 的 getsignatureinfo 以探测签名协议
	q := u.Query()
	q.Set("action", "gatewayservice")
	q.Set("method", "getsignatureinfo")
	q.Set("format", "json")
	u.RawQuery = q.Encode()
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return failure.Wrap("wiki.discovery.prepare", err)
	}
	res, err := c.HTTP.Do(r)
	if err != nil {
		return failure.Dependency("wiki.discovery.request", err)
	}
	defer res.Body.Close()
	var v struct {
		Gateway struct {
			Info json.RawMessage `json:"signatureinfo"`
		} `json:"gatewayservice"`
	}
	if res.StatusCode != 200 {
		return &failure.Error{Code: failure.Upstream, Message: "Bridge signature discovery failed.", Op: "wiki.discovery.response", UpstreamStatus: res.StatusCode, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	if err := decodeResponse(res.Body, &v); err != nil {
		return failure.Dependency("wiki.discovery.decode", err)
	}
	if len(v.Gateway.Info) == 0 {
		return failure.New(failure.Upstream, "Bridge signature discovery is unavailable.", "wiki.discovery.validate", nil)
	}
	var info struct {
		Version   int    `json:"version"`
		Protocol  string `json:"protocol"`
		Algorithm string `json:"algorithm"`
		Label     string `json:"label"`
	}
	// 仅接受 RFC 9421 + hmac-sha256 + sig1 + version 1 的兼容协议
	if json.Unmarshal(v.Gateway.Info, &info) != nil || info.Version != 1 || info.Protocol != "RFC 9421" || info.Algorithm != "hmac-sha256" || info.Label != "sig1" {
		return failure.New("WIKI_CAPABILITY_UNAVAILABLE", "Bridge signature protocol is incompatible.", "wiki.discovery.validate", nil)
	}
	return nil
}
