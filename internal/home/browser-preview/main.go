// Browser test fixture: serves the real homepage and resources without external services.
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"isekai-mcp-gateway/internal/assets"
	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/home"
	"isekai-mcp-gateway/internal/httpserver/middleware"
	"isekai-mcp-gateway/internal/templates"
)

func main() {
	c := config.Defaults()
	c.Static.Auth.Gateway.Resource = "https://wiki.example/mcp?test=1&mode=read"
	if value := os.Getenv("HOME_TEST_ENDPOINT"); value != "" {
		c.Static.Auth.Gateway.Resource = value
	}
	pages, err := templates.New(".")
	if err != nil {
		panic(err)
	}
	files, err := assets.New(".")
	if err != nil {
		panic(err)
	}
	r := chi.NewRouter()
	(&home.Service{Config: c, Pages: pages, Site: func() (string, string) { return "异世界百科", "" }}).Routes(r)
	files.Routes(r)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Printf("http://%s\n", listener.Addr())
	if err := http.Serve(listener, middleware.Headers(r)); err != nil {
		panic(err)
	}
}
