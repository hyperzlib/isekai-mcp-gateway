package gatewaycli

import (
	"context"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/bootstrap"
	"isekai-mcp-gateway/internal/failure"
	"isekai-mcp-gateway/internal/runtimefiles"
)

func serveCommand() *cli.Command {
	return &cli.Command{Name: "serve", Usage: "Start the gateway server", Flags: []cli.Flag{&cli.BoolFlag{Name: "watch-templates", Value: true, Usage: "Reload templates when directory files change"}}, Action: func(ctx context.Context, cmd *cli.Command) error {
		files, err := runtimefiles.Load(cmd.String("data-dir"))
		if err != nil {
			err = failure.Wrap("startup.files", err)
			failure.Log(ctx, "startup.files", err)
			return err
		}
		c, err := loadConfig(cmd)
		if err != nil {
			failure.Log(ctx, "startup.config", err)
			return err
		}
		err = bootstrap.Serve(c, files, cmd.Bool("watch-templates"))
		if err != nil {
			err = failure.Wrap("startup.serve", err)
			failure.Log(ctx, "startup.serve", err)
		}
		return err
	}}
}
