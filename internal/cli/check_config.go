package gatewaycli

import (
	"context"
	"fmt"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/runtimefiles"
)

func checkConfigCommand() *cli.Command {
	return &cli.Command{Name: "check-config", Usage: "Validate configuration offline", Action: checkConfig}
}
func checkConfig(_ context.Context, cmd *cli.Command) error {
	if _, err := runtimefiles.Load(cmd.String("data-dir")); err != nil {
		return err
	}
	if _, err := loadConfig(cmd); err != nil {
		return err
	}
	_, err := fmt.Fprintln(cmd.Root().Writer, "Configuration is valid; no connections were attempted.")
	return err
}
