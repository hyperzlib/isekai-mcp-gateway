package gatewaymcp

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewTransport(maxBytes int64, definitions []Definition) (*mcp.StreamableHTTPHandler, error) {
	newServer := func() (*mcp.Server, error) {
		s := mcp.NewServer(&mcp.Implementation{Name: "isekai-mcp-gateway", Version: "0.1.0"},
			&mcp.ServerOptions{Instructions: "Use wiki.help for available capabilities."})
		if err := Register(s, definitions); err != nil {
			return nil, err
		}
		return s, nil
	}

	// 启动时先构造一次，尽早暴露定义注册错误。
	if _, err := newServer(); err != nil {
		return nil, err
	}

	// 每个请求按需新建 Server，无跨请求共享状态；同时限制请求体大小与空闲会话存活时间。
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { s, _ := newServer(); return s },
		&mcp.StreamableHTTPOptions{
			SessionTimeout:               30 * time.Minute,
			MaxRequestBodyBytes:          maxBytes,
			DisableLocalhostProtection:   true,
			PropagateRequestCancellation: true,
			Stateless:                    true,
		}), nil
}
