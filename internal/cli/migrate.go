package gatewaycli

import (
	"context"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/bootstrap"
)

func migrateCommand() *cli.Command {
	return &cli.Command{Name: "migrate", Usage: "Migrate the gateway database", Action: func(ctx context.Context, cmd *cli.Command) error {
		c, err := loadConfig(cmd)
		if err != nil {
			return err
		}
		return bootstrap.Migrate(ctx, c)
	}}
}
