package bootstrap

import (
	"github.com/go-chi/chi/v5"

	"isekai-mcp-gateway/internal/auth/provider/zitadel"
	"isekai-mcp-gateway/internal/auth/resource"
	"isekai-mcp-gateway/internal/auth/web"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/httpserver"
	"isekai-mcp-gateway/internal/httpserver/health"
	gatewaymcp "isekai-mcp-gateway/internal/mcp"
	"isekai-mcp-gateway/internal/mcp/modules/hello"
	"isekai-mcp-gateway/internal/mcp/modules/help"
	"isekai-mcp-gateway/internal/mcp/modules/logout"
	"isekai-mcp-gateway/internal/mcp/modules/me"
	"isekai-mcp-gateway/internal/mcp/modules/wikiactivity"
	"isekai-mcp-gateway/internal/mcp/modules/wikiedit"
	"isekai-mcp-gateway/internal/mcp/modules/wikimutation"
	"isekai-mcp-gateway/internal/mcp/modules/wikiread"
	"isekai-mcp-gateway/internal/runtimefiles"
	"isekai-mcp-gateway/internal/wiki"
)

type routeFunc func(chi.Router)

func (fn routeFunc) Routes(r chi.Router) { fn(r) }

func assemble(c *config.Config, a *zitadel.Service, files *runtimefiles.Files, business *wiki.Service) (*httpserver.Server, error) {
	pages := web.New(a, files.Templates)
	resourceServer := &resource.Service{Service: a.Service}
	authorizer := &gatewaymcp.Authenticator{Service: a.Service}

	// 汇集所有 MCP 模块的工具定义
	definitions := append(help.Module(authorizer, files.Templates), hello.Module(authorizer)...)
	definitions = append(definitions, logout.Module(authorizer)...)
	definitions = append(definitions, me.Module(authorizer)...)
	definitions = append(definitions, wikiread.Module(business)...)
	definitions = append(definitions, wikiactivity.Module(business)...)
	definitions = append(definitions, wikiedit.Module(business)...)
	definitions = append(definitions, wikimutation.Module(business)...)
	stream, err := gatewaymcp.NewTransport(c.Static.Server.RequestBodyMaxBytes, definitions)
	if err != nil {
		return nil, err
	}

	// 就绪探测同时检查依赖健康与出站配置是否已加载
	status := &health.Handler{Dependencies: a.Service, OutboundReady: func() bool { return a.Upstream.Get() != nil }}
	server := httpserver.New(c, a, resourceServer, business,
		routeFunc(func(r chi.Router) { pages.Routes(r) }), files.Assets,
		&gatewaymcp.Transport{Handler: resourceServer.Bearer(gatewaymcp.BearerMetadata(c.Static.Auth.Gateway.Issuer, stream))}, status)
	server.MCP = stream
	return server, nil
}
