// Package home serves the public MCP connection guide.
package home

import (
	"bytes"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/templates"
)

type Page struct {
	SiteTitle, SiteIconURL, Endpoint, Title string
	CloudWarning                            bool
}
type Service struct {
	Config *config.Config
	Pages  templates.HTMLRenderer
	Site   func() (string, string)
}

func (s *Service) Routes(r chi.Router) { r.Get("/", s.serve) }
func cloudWarning(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return true
	}
	host := strings.ToLower(u.Hostname())
	ip := net.ParseIP(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || (ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()))
}
func (s *Service) serve(w http.ResponseWriter, r *http.Request) {
	title, icon := s.Site()
	if strings.TrimSpace(title) == "" {
		title = config.DefaultSiteTitle
	}
	endpoint := s.Config.Static.Auth.Gateway.Resource
	p := Page{SiteTitle: title, SiteIconURL: icon, Endpoint: endpoint, Title: title + " · MCP 接入指南", CloudWarning: cloudWarning(endpoint)}
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
