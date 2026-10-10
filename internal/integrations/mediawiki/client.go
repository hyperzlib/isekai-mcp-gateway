package mediawiki

import (
	"bytes"
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
	// traceFailed 在身份解析失败时记录脱敏后的请求与 MediaWiki API 响应，
	// request_id 与错误页展示的错误编号一致，便于关联排查。
	traceFailed := func(err error, status int, response []byte) {
		failure.TraceUpstream(ctx, "wiki.identity.request", failure.UpstreamTrace{
			Method:         req.Method,
			URL:            req.URL.String(),
			RequestHeaders: failure.FormatRequestHeaders(req, "Signature", "Authorization", "X-Isekai-User-Token", "Cookie"),
			RequestBody:    body,
			Status:         status,
			ResponseBody:   failure.ClipResponseBody(response),
		}, err)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		err = failure.Dependency("wiki.identity.request", err)
		traceFailed(err, 0, nil)
		return Identity{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil {
		err = failure.Dependency("wiki.identity.decode", err)
		traceFailed(err, res.StatusCode, nil)
		return Identity{}, err
	}
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
		err = &failure.Error{Code: failure.Upstream, Message: "Wiki 身份服务请求失败。", Op: "wiki.identity.response", Retryable: res.StatusCode == 429 || res.StatusCode >= 500, UpstreamStatus: res.StatusCode}
		traceFailed(err, res.StatusCode, data)
		return Identity{}, err
	}
	if err := decodeResponse(bytes.NewReader(data), &v); err != nil {
		err = failure.Dependency("wiki.identity.decode", err)
		traceFailed(err, res.StatusCode, data)
		return Identity{}, err
	}
	if v.Error != nil {
		err := identityError(v.Error.Code)
		traceFailed(err, res.StatusCode, data)
		return Identity{}, err
	}

	// 用户信息字段必须完整
	out := v.Gateway.User
	if out.UserID <= 0 || out.UserToken == "" || out.Username == "" {
		err := failure.New(failure.Upstream, "Wiki 身份服务返回的用户信息不完整。", "wiki.identity.validate", nil)
		traceFailed(err, res.StatusCode, data)
		return Identity{}, err
	}
	return out, nil
}

// decodeResponse enforces the byte limit on the entire response, not only a
// successfully decoded JSON prefix。身份载荷仅用于解析不进入错误对象；
// 失败的响应体由调用方截断后经 failure 的脱敏日志输出。
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
