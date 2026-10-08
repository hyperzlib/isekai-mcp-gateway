package gatewaycli

import (
	"context"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/config"
)

func writeSchemaCommand() *cli.Command {
	return &cli.Command{Name: "write-schema", Usage: "Generate JSON schemas in docs", Action: writeSchema}
}
func writeSchema(_ context.Context, _ *cli.Command) error { return config.GenerateSchemas("docs") }
