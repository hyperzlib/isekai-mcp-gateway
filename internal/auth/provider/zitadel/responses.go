package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/httpresponse"
)

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	httpresponse.JSON(r.Context(), w, status, value)
}

// OAuth responses are bounded JSON or redirects. Buffer only these routes;
// MCP remains a direct streaming handler.
type responseBuffer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponseBuffer() *responseBuffer { return &responseBuffer{header: make(http.Header)} }

func (b *responseBuffer) Header() http.Header { return b.header }

func (b *responseBuffer) WriteHeader(n int) {
	if b.status == 0 {
		b.status = n
	}
}

func (b *responseBuffer) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = 200
	}
	return b.body.Write(p)
}

// flush 将缓冲的响应一次性写入底层 ResponseWriter，确保提交前完成改写
func (b *responseBuffer) flush(ctx context.Context, w http.ResponseWriter) {
	for k, values := range b.header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	if b.status == 0 {
		b.status = 200
	}
	w.WriteHeader(b.status)
	if _, err := w.Write(b.body.Bytes()); err != nil {
		failure.Log(ctx, "oauth.write", failure.Wrap("oauth.write", err))
	}
}

func (s *Service) AuthError(w http.ResponseWriter, r *http.Request, redirect, state, code, description string) {
	u, err := url.Parse(redirect)
	if err != nil {
		failure.Log(r.Context(), "oauth.redirect", failure.Wrap("oauth.redirect", err))
		writeJSON(w, r, 500, map[string]string{"error": "server_error"})
		return
	}
	q := u.Query()
	q.Set("error", code)
	q.Set("error_description", description)
	q.Set("iss", s.Config.Static.Auth.Gateway.Issuer)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Service) providerResponse(w http.ResponseWriter, r *http.Request, token bool) {
	s.providerResponseWith(w, r, token, s.Provider)
}

func (s *Service) providerResponseWith(w http.ResponseWriter, r *http.Request, token bool, provider http.Handler) {
	// 在缓冲中执行 provider，便于提交前检查失败边界并改写响应
	ctx := failure.WithBoundary(r.Context())
	r = r.WithContext(ctx)
	b := newResponseBuffer()
	provider.ServeHTTP(b, r)
	if err := failure.BoundaryFailure(ctx); err != nil {
		// Provider internals may have swallowed or reclassified storage errors.
		// Discard that response before commitment and send a safe server error.
		writeJSON(w, r, 500, map[string]string{"error": "server_error", "error_description": "Service operation failed."})
		return
	}
	// 授权响应重定向需补发 iss 参数（RFC 9207）
	if location := b.header.Get("Location"); location != "" {
		u, err := url.Parse(location)
		if err != nil {
			failure.Log(ctx, "oauth.redirect_rewrite", failure.Wrap("oauth.redirect_rewrite", err))
			writeJSON(w, r, 500, map[string]string{"error": "server_error"})
			return
		}
		if err == nil && (u.Query().Has("code") || u.Query().Has("error")) {
			q := u.Query()
			q.Set("iss", s.Config.Static.Auth.Gateway.Issuer)
			u.RawQuery = q.Encode()
			b.header.Set("Location", u.String())
		}
	}
	// 令牌端点响应必须改写：剥离内部 id_token 并保证 JSON 对象结构
	if token && b.status == 200 {
		var v map[string]json.RawMessage
		if err := json.Unmarshal(b.body.Bytes(), &v); err != nil || v == nil {
			if err == nil {
				err = errors.New("token response must be an object")
			}
			failure.Log(r.Context(), "oauth.rewrite", failure.Wrap("oauth.rewrite", err))
			writeJSON(w, r, 500, map[string]string{"error": "server_error"})
			return
		} else {
			// The Local IDP serves OAuth access to Agents. Its internal OIDC ID
			// token is not needed by clients and must not become a bearer token.
			delete(v, "id_token")
			b.body.Reset()
			if err := json.NewEncoder(&b.body).Encode(v); err != nil {
				failure.Log(r.Context(), "oauth.rewrite", failure.Wrap("oauth.rewrite", err))
				writeJSON(w, r, 500, map[string]string{"error": "server_error"})
				return
			}
			b.header.Del("Content-Length")
		}
	}
	b.header.Set("Cache-Control", "no-store")
	b.flush(r.Context(), w)
}
