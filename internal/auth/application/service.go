package application

import (
	"context"
	"time"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/outbound"
)

type RequestKey struct{}

type RequestInfo struct {
	Browser, Resource, ClientID, Code string
	RefreshScopesPresent              bool
	RefreshScopes                     []string
}

type Service struct {
	Config        *config.Config
	Store         RecordStore
	Upstream      *outbound.Manager
	Decoder       TokenDecoder
	FetchMetadata MetadataFetcher
}

func (s *Service) Health(ctx context.Context) error { return s.Store.Health(ctx) }

func Seconds(n int) time.Duration { return time.Duration(n) * time.Second }
