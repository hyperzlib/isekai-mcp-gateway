package httpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	Handler http.Handler
	MCP     *mcp.StreamableHTTPHandler
}
