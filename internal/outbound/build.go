package outbound

import (
	"context"
	"net/http"
	"time"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/integrations/oidcclient"
)

func Build(ctx context.Context, c *config.Config) (*Bundle, error) {
	h := &http.Client{Timeout: 15 * time.Second}
	oidcClient, err := oidcclient.New(ctx, c, h)
	if err != nil {
		return nil, err
	}
	b := mediawiki.New(c)
	if err = b.Probe(ctx); err != nil {
		return nil, err
	}
	if err = b.Capabilities(ctx, c); err != nil {
		return nil, err
	}
	return &Bundle{Config: c, OidcClient: oidcClient, Bridge: b, HTTP: h}, nil
}
