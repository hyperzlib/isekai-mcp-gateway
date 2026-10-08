package gatewaycli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/assets"
	"isekai-mcp-gateway/internal/filebundle"
	"isekai-mcp-gateway/internal/templates"
)

func exportTemplatesCommand() *cli.Command {
	return exportCommand("export-templates", "Export built-in HTML and text templates", templates.Export)
}

func exportAssetsCommand() *cli.Command {
	return exportCommand("export-assets", "Export built-in static resources", assets.Export)
}

func exportCommand(name, usage string, export func(string, bool) (filebundle.ExportResult, error)) *cli.Command {
	return &cli.Command{Name: name, Usage: usage,
		Flags: []cli.Flag{&cli.BoolFlag{Name: "overwrite", Usage: "Overwrite existing regular files"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			result, err := export(cmd.String("data-dir"), cmd.Bool("overwrite"))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.Root().Writer, "Exported %d files; skipped %d existing files.\n", result.Written, result.Skipped)
			return err
		},
	}
}
