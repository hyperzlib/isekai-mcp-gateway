package mediawiki

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/integrations/mediawiki/servicebridge"
)

type Credentials struct {
	UserID int64
	Token  string
}
type APIError struct {
	Code string
	Data map[string]any
}

func (e *APIError) Error() string { return "MediaWiki rejected the operation" }
func (e *APIError) Failure() *failure.Error {
	code := "UPSTREAM_ERROR"

	// 将 MediaWiki 错误码映射为网关统一错误码
	switch e.Code {
	case "permissiondenied", "badaccess-group0", "protectedpage":
		code = "PERMISSION_DENIED"
	case "missingtitle", "nosuchrevid", "invalidtitle":
		code = "NOT_FOUND"
	case "editconflict", "moderation-editconflict":
		code = "EDIT_CONFLICT"
	case "articleexists":
		code = "CREATE_CONFLICT"
	case "badtoken":
		code = "CSRF_TOKEN_INVALID"
	case "maxlag", "ratelimited":
		code = "UPSTREAM_RATE_LIMITED"
	}
	if strings.HasPrefix(e.Code, "isekai-servicebridge-") {
		code = "INVALID_TOKEN"
	}
	return &failure.Error{Code: code, Message: "MediaWiki rejected the operation. Read the current state before retrying.", Op: "wiki.api", Details: map[string]any{"upstream_code": e.Code}}
}

// csrfTokens 按站点/keyId/用户缓存 CSRF token
var csrfTokens sync.Map

func tokenKey(c *Client, cred Credentials) string {
	// token 哈希后参与缓存键，避免明文长期驻留
	h := sha256.Sum256([]byte(cred.Token))
	return fmt.Sprintf("%s:%s:%d:%x", c.Config.SiteID, c.Config.Bridge.KeyID, cred.UserID, h)
}
func (c *Client) CSRF(ctx context.Context, cred Credentials) (string, error) {
	key := tokenKey(c, cred)

	// 命中缓存直接返回
	if v, ok := csrfTokens.Load(key); ok {
		return v.(string), nil
	}
	v, e := c.Query(ctx, cred, url.Values{"action": {"query"}, "meta": {"tokens"}, "type": {"csrf"}})
	if e != nil {
		return "", e
	}
	q, _ := v["query"].(map[string]any)
	ts, _ := q["tokens"].(map[string]any)
	t, _ := ts["csrftoken"].(string)

	// 空 token 或匿名占位符 "+\\" 视为无效
	if t == "" || t == "+\\" {
		return "", failure.New("UPSTREAM_RESPONSE_INVALID", "Wiki did not return a CSRF token.", "wiki.csrf", nil)
	}
	csrfTokens.Store(key, t)
	return t, nil
}
func (c *Client) Query(ctx context.Context, cred Credentials, params url.Values) (map[string]any, error) {
	return c.call(ctx, cred, params, false, nil, "", "")
}
func (c *Client) Write(ctx context.Context, cred Credentials, params url.Values) (map[string]any, error) {
	return c.call(ctx, cred, params, true, nil, "", "")
}

// Upload signs the same multipart bytes it sends. Only scalar fields are accepted.
func (c *Client) Upload(ctx context.Context, cred Credentials, params url.Values, file []byte, filename, mime string) (map[string]any, error) {
	return c.call(ctx, cred, params, true, file, filename, mime)
}
func (c *Client) call(ctx context.Context, cred Credentials, p url.Values, write bool, file []byte, filename, mime string) (map[string]any, error) {
	// 复制参数，避免修改调用方传入的 url.Values
	original := p
	p = make(url.Values, len(original))
	for k, v := range original {
		p[k] = append([]string(nil), v...)
	}
	p.Set("format", "json")
	p.Set("formatversion", "2")
	if write {
		p.Set("maxlag", strconv.Itoa(c.Config.Maxlag))
	}

	// 写操作走 POST，读操作走 GET
	method, target, body, contentType := "GET", c.Config.APIURL, []byte(nil), ""
	digest := ""
	if write {
		method = "POST"
		contentType = "application/x-www-form-urlencoded"
		body = []byte(p.Encode())
		if file != nil {
			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			var records []string
			keys := make([]string, 0, len(p))
			for k := range p {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			// 仅接受标量字段，按 key 排序保证签名可重现
			for _, k := range keys {
				if len(p[k]) != 1 {
					return nil, failure.New("INVALID_REQUEST", "Multipart fields must be scalar.", "wiki.upload", nil)
				}
				v := p.Get(k)
				if err := w.WriteField(k, v); err != nil {
					return nil, err
				}
				h := sha256.Sum256([]byte(v))
				records = append(records, url.QueryEscape("text:/"+k)+"="+hex.EncodeToString(h[:]))
			}
			part, e := w.CreateFormFile("file", filename)
			if e != nil {
				return nil, e
			}
			if _, e = part.Write(file); e != nil {
				return nil, e
			}
			if e = w.Close(); e != nil {
				return nil, e
			}

			// 文件摘要与字段摘要合并后再哈希，形成 multipart digest
			h := sha256.Sum256(file)
			records = append(records, url.QueryEscape("file:/file")+"="+hex.EncodeToString(h[:]))
			sort.Strings(records)
			dh := sha256.Sum256([]byte(strings.Join(records, ";")))
			digest = "v1=:" + base64.StdEncoding.EncodeToString(dh[:]) + ":"
			body = b.Bytes()
			contentType = w.FormDataContentType()
		}
	} else {
		// GET 将参数编码进查询串
		u, e := url.Parse(target)
		if e != nil {
			return nil, e
		}
		q := u.Query()
		for k, v := range p {
			q[k] = v
		}
		u.RawQuery = q.Encode()
		target = u.String()
	}
	req, e := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if e != nil {
		return nil, failure.Wrap("wiki.api.prepare", e)
	}

	// 附带用户身份与 Bridge 签名头
	req.Header.Set("X-Isekai-User-Id", strconv.FormatInt(cred.UserID, 10))
	req.Header.Set("X-Isekai-User-Token", cred.Token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if digest != "" {
		req.Header.Set("X-Isekai-Multipart-Digest", digest)
	}
	if e = servicebridge.Sign(req, body, c.Config.Bridge.KeyID, c.Secret); e != nil {
		return nil, e
	}
	res, e := c.HTTP.Do(req)
	if e != nil {
		return nil, &failure.Error{Code: "API_NETWORK_ERROR", Message: "MediaWiki API network request failed. The request was not automatically retried. Inspect the operation state before taking further action.", Op: "wiki.api.request", Cause: e}
	}
	defer res.Body.Close()

	// 限制响应 32MiB 并解析 JSON
	data, e := io.ReadAll(io.LimitReader(res.Body, 32*1024*1024+1))
	if e != nil {
		return nil, failure.New("API_NETWORK_ERROR", "MediaWiki API response was interrupted. Inspect the operation state.", "wiki.api.read", e)
	}
	if len(data) > 32*1024*1024 {
		return nil, failure.New("UPSTREAM_RESPONSE_TOO_LARGE", "Wiki response exceeds the limit.", "wiki.api.read", nil)
	}
	var v map[string]any
	if json.Unmarshal(data, &v) != nil || v == nil {
		return nil, failure.New("UPSTREAM_RESPONSE_INVALID", "Wiki returned an invalid response.", "wiki.api.decode", nil)
	}

	// 上游返回 error：badtoken/桥接错误清缓存；moderation 排队视为成功
	if a, ok := v["error"].(map[string]any); ok {
		code, _ := a["code"].(string)
		if code == "badtoken" {
			csrfTokens.Delete(tokenKey(c, cred))
		}
		if strings.HasPrefix(code, "isekai-servicebridge-") {
			csrfTokens.Delete(tokenKey(c, cred))
		}
		if code == "moderation-edit-queued" {
			return v, nil
		}
		return nil, &APIError{Code: code, Data: a}
	}
	if res.StatusCode != 200 {
		return nil, &failure.Error{Code: "UPSTREAM_ERROR", Message: "Wiki HTTP request failed.", Op: "wiki.api.response", UpstreamStatus: res.StatusCode}
	}
	return v, nil
}
