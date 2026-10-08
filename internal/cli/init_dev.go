package gatewaycli

import (
	"context"
	"path/filepath"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/config"
)

func initDevCommand() *cli.Command {
	return &cli.Command{Name: "init-dev", Usage: "Create development configuration", Action: func(_ context.Context, cmd *cli.Command) error {
		dir, err := filepath.Abs(cmd.String("config-dir"))
		if err != nil {
			return err
		}
		return config.InitDevelopment(dir)
	}}
}
