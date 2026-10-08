package bootstrap

import (
	"context"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/persistence"
)

func Migrate(ctx context.Context, c *config.Config) error {
	store, err := persistence.Open(c)
	if err != nil {
		return err
	}
	defer store.SQL.Close()
	return store.Migrate(ctx)
}
