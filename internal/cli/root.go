package gatewaycli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/auth/provider/zitadel"
	"isekai-mcp-gateway/internal/config"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	root := &cli.Command{Name: "gateway", Usage: "Isekai MCP Gateway", Writer: stdout, ErrWriter: stderr,
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		OnUsageError:   func(_ context.Context, _ *cli.Command, err error, _ bool) error { return err },
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config-dir", Value: "config", Usage: "Configuration directory"},
			&cli.StringFlag{Name: "data-dir", Value: ".", Usage: "Runtime templates and assets directory"},
			&cli.BoolFlag{Name: "check-config", Usage: "Alias for check-config action"},
			&cli.BoolFlag{Name: "write-schema", Usage: "Alias for write-schema action"},
		},
	}
	root.Commands = []*cli.Command{serveCommand(), migrateCommand(), initDevCommand(), checkConfigCommand(), writeSchemaCommand(), generateKeysCommand(), exportTemplatesCommand(), exportAssetsCommand()}

	// 包装每个子命令的 Action，统一拦截旧式动作标志与位置参数
	for _, command := range root.Commands {
		action := command.Action
		command.Action = func(ctx context.Context, cmd *cli.Command) error {
			if cmd.IsSet("check-config") || cmd.IsSet("write-schema") {
				return fmt.Errorf("legacy action flags cannot be combined with an explicit action")
			}
			if cmd.NArg() != 0 {
				return fmt.Errorf("%s accepts no positional arguments", cmd.Name)
			}
			return action(ctx, cmd)
		}
	}

	// 根命令无显式动作时，按旧式标志分派 check-config/write-schema，否则展示帮助
	root.Action = func(ctx context.Context, cmd *cli.Command) error {
		if cmd.NArg() != 0 {
			return fmt.Errorf("unknown action: %s", cmd.Args().First())
		}
		if cmd.IsSet("check-config") && cmd.IsSet("write-schema") {
			return fmt.Errorf("--check-config and --write-schema cannot be combined")
		}
		if cmd.Bool("check-config") {
			return checkConfig(ctx, cmd)
		}
		if cmd.Bool("write-schema") {
			return writeSchema(ctx, cmd)
		}
		return cli.ShowAppHelp(cmd)
	}
	return root.Run(ctx, args)
}

func loadConfig(cmd *cli.Command) (*config.Config, error) {
	dir, err := filepath.Abs(cmd.String("config-dir"))
	if err != nil {
		return nil, err
	}
	c, err := config.Load(dir, nil)
	if err != nil {
		return nil, err
	}

	// 额外实例化 zitadel，在加载阶段提前暴露客户端配置错误
	if _, err := zitadel.New(c, nil, nil); err != nil {
		return nil, err
	}
	return c, nil
}
