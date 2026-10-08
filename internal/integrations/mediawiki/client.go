package mediawiki

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki/servicebridge"
)

type Client struct {
	Config config.MediaWiki
	Secret string
	HTTP   *http.Client
}

func New(c *config.Config) *Client {
	// 禁止跟随 Bridge 重定向，避免签名凭证被转发到其他主机
	return &Client{Config: c.App.MediaWiki, Secret: c.Secrets.BridgeSecret, HTTP: &http.Client{
		Timeout:       time.Duration(c.App.MediaWiki.TimeoutMS) * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Bridge redirects are prohibited") },
	}}
}

func (c *Client) Resolve(ctx context.Context, sub string) (Identity, error) {
	// 以表单形式调用 Bridge 的 oidcgetuserinfo
	body := url.Values{"action": {"gatewayservice"}, "method": {"oidcgetuserinfo"}, "format": {"json"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, "POST", c.Config.APIURL, strings.NewReader(body))
	if err != nil {
		return Identity{}, failure.Wrap("wiki.identity.prepare", err)
	}

	// 附带用户身份头并用 Bridge 密钥签名
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Isekai-User-Id", "0")
	req.Header.Set("X-Isekai-Subject", sub)
	if err = servicebridge.Sign(req, []byte(body), c.Config.Bridge.KeyID, c.Secret); err != nil {
		return Identity{}, failure.Wrap("wiki.identity.prepare", err)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Identity{}, failure.Dependency("wiki.identity.request", err)
	}
	defer res.Body.Close()
	var v struct {
		Gateway struct {
			User Identity `json:"userinfo"`
		} `json:"gatewayservice"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	// 上游非 200 归类为上游错误，429/5xx 可重试
	if res.StatusCode != 200 {
		return Identity{}, &failure.Error{Code: failure.Upstream, Message: "Wiki 身份服务请求失败。", Op: "wiki.identity.response", Retryable: res.StatusCode == 429 || res.StatusCode >= 500, UpstreamStatus: res.StatusCode}
	}
	if err := decodeResponse(res.Body, &v); err != nil {
		return Identity{}, failure.Dependency("wiki.identity.decode", err)
	}
	if v.Error != nil {
		return Identity{}, identityError(v.Error.Code)
	}

	// 用户信息字段必须完整
	out := v.Gateway.User
	if out.UserID <= 0 || out.UserToken == "" || out.Username == "" {
		return Identity{}, failure.New(failure.Upstream, "Wiki 身份服务返回的用户信息不完整。", "wiki.identity.validate", nil)
	}
	return out, nil
}

// decodeResponse enforces the byte limit on the entire response, not only a
// successfully decoded JSON prefix. Never retain or expose upstream payloads.
func decodeResponse(body io.Reader, value any) error {
	// 多读 1 字节以检测越过上限的响应
	data, err := io.ReadAll(io.LimitReader(body, 65537))
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return errors.New("Wiki response exceeds byte limit")
	}
	return json.Unmarshal(data, value)
}
