package gatewaymcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isekai-mcp-gateway/internal/auth/application"
	"isekai-mcp-gateway/internal/auth/domain"
)

type RequestUserReader interface {
	MCPCurrentUser(context.Context, *mcp.RequestExtra) (domain.UserInfo, error)
}

func (s *Authenticator) MCPCurrentUser(ctx context.Context, extra *mcp.RequestExtra) (domain.UserInfo, error) {
	if err := s.CheckMCPRequest(ctx, extra); err != nil {
		return domain.UserInfo{}, err
	}
	return s.CurrentUser(ctx, extra.TokenInfo.Extra["principal"].(*application.Principal))
}
