package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
	"isekai-mcp-gateway/internal/security"
)

func (s *Service) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: strings.HasPrefix(s.Config.Static.Auth.Gateway.Issuer, "https://"), HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

// browser 为每个浏览器签发带 HMAC 签名的稳定标识，用于绑定授权事务
func (s *Service) browser(w http.ResponseWriter, r *http.Request) string {
	// 使用域分隔前缀对随机 id 做 HMAC，防止签名跨用途复用
	mac := func(raw string) string {
		h := hmac.New(sha256.New, s.Config.Secrets.Cookie)
		h.Write([]byte("gateway-browser:" + raw))
		return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	}

	// 已有合法签名 Cookie 时直接复用，否则重新签发
	if c, err := r.Cookie("gateway_browser"); err == nil {
		id, signature, ok := strings.Cut(c.Value, ".")
		if ok && len(id) == 43 && hmac.Equal([]byte(signature), []byte(mac(id))) {
			return security.Hash(id)
		}
	}
	id := security.Random()
	s.cookie(w, "gateway_browser", id+"."+mac(id), s.Config.Static.Auth.Gateway.GrantTTL)
	return security.Hash(id)
}

func (s *Service) browserSession(r *http.Request, browser string) (*domain.Session, error) {
	c, err := r.Cookie("gateway_session")
	if err != nil {
		return nil, application.ErrNoSession
	}
	return s.LoadBrowserSession(r.Context(), c.Value, browser)
}
