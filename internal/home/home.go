// Package home serves the public MCP connection guide.
package home

import (
	"bytes"
	"html"
	htmltemplate "html/template"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/justinas/nosurf"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/templates"
)

type Page struct {
	SiteTitle, SiteIconURL, SiteUrl, Endpoint, Title string
	CloudWarning                                     bool
	DiscoveryComment                                 htmltemplate.HTML
	HeaderLink                                       *config.SiteHeaderLink
	// 登录态注入：已登录时导航展示账号入口与 API Keys 链接。
	LoggedIn                                     bool
	Username, AvatarURL, Initial, CSRF, ReturnTo string
	KeyEnabled                                   bool
}

// LoginState 由 web 层注入：探测浏览器会话并返回登录态与账号信息。
type LoginState func(http.ResponseWriter, *http.Request) (loggedIn bool, username, avatar string)

type Service struct {
	Config *config.Config
	Pages  interface {
		templates.HTMLRenderer
		templates.TextRenderer
	}
	Site       func() (string, string)
	HeaderLink func() *config.SiteHeaderLink
	Login      LoginState
}

func (s *Service) Routes(r chi.Router) {
	csrf := nosurf.New(http.HandlerFunc(s.serve))
	csrf.SetBaseCookie(http.Cookie{Name: "gateway_csrf", Path: "/auth", HttpOnly: true, Secure: strings.HasPrefix(s.Config.Static.Auth.Gateway.Issuer, "https://"), SameSite: http.SameSiteLaxMode})
	r.Method(http.MethodGet, "/", csrf)
	r.Get("/llms.txt", s.llms)
}
func cloudWarning(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return true
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()))
}
func (s *Service) discovery() (string, error) {
	title, _ := s.Site()
	if strings.TrimSpace(title) == "" {
		title = config.DefaultSiteTitle
	}
	var out bytes.Buffer
	err := s.Pages.RenderText(&out, "discovery.md", Page{SiteTitle: title, SiteUrl: strings.TrimRight(s.Config.Static.Server.PublicURL, "/"), Endpoint: s.Config.Static.Auth.Gateway.Resource})
	return out.String(), err
}

func (s *Service) llms(w http.ResponseWriter, r *http.Request) {
	content, err := s.discovery()
	if err != nil {
		http.Error(w, "Unable to render discovery", 500)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(content))
}

func (s *Service) serve(w http.ResponseWriter, r *http.Request) {
	title, icon := s.Site()
	if strings.TrimSpace(title) == "" {
		title = config.DefaultSiteTitle
	}
	endpoint := s.Config.Static.Auth.Gateway.Resource
	p := Page{SiteTitle: title, SiteIconURL: icon, Endpoint: endpoint, Title: title + " · MCP 接入指南", CloudWarning: cloudWarning(endpoint)}
	discovery, err := s.discovery()
	if err != nil {
		http.Error(w, "Unable to render discovery", 500)
		return
	}
	// Escape markup and comment delimiters before embedding untrusted configuration.
	safe := strings.ReplaceAll(html.EscapeString(discovery), "--", "&#45;&#45;")
	p.DiscoveryComment = htmltemplate.HTML("<!-- " + safe + " -->")
	w.Header().Set("Link", `</llms.txt>; rel="describedby"; type="text/markdown"`)
	if s.HeaderLink != nil {
		p.HeaderLink = s.HeaderLink()
	}
	// 探测已保存登录态，注入首页导航；未登录时保持原有公开页面。
	if s.Login != nil {
		p.LoggedIn, p.Username, p.AvatarURL = s.Login(w, r)
		if p.LoggedIn {
			p.KeyEnabled = s.Config.Static.Auth.Gateway.APIKeys.Enabled
			p.CSRF = nosurf.Token(r)
			p.ReturnTo = "/"
			if p.Username == "" {
				p.Username = p.AvatarURL
			}
			if p.Username != "" {
				p.Initial = string([]rune(p.Username)[:1])
			}
			// 头像缺失时回退配置的默认头像。
			if p.AvatarURL == "" {
				p.AvatarURL = s.Config.App.Auth.OidcClient.DefaultAvatar
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	var out bytes.Buffer
	if err := s.Pages.RenderHTML(&out, "home", p); err != nil {
		failure.Log(r.Context(), "page.render", failure.Wrap("page.render", err))
		http.Error(w, "Unable to render page", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = out.WriteTo(w)
}
