package outbound

import (
	"net/http"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/integrations/mediawiki"
	"isekai-mcp-gateway/internal/integrations/oidcclient"
)

// Bundle freezes configuration and outbound clients for an entire login.
type Bundle struct {
	Config     *config.Config
	OidcClient *oidcclient.OidcConnect
	Bridge     *mediawiki.Client
	HTTP       *http.Client
}
