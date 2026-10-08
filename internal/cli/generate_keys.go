package gatewaycli

import (
	"context"
	"encoding/json"

	cli "github.com/urfave/cli/v3"

	"isekai-mcp-gateway/internal/security"
)

func generateKeysCommand() *cli.Command {
	return &cli.Command{Name: "generate-keys", Usage: "Print newly generated keys as JSON", Action: func(_ context.Context, cmd *cli.Command) error {
		keys, err := security.GenerateKeys()
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(keys, "", "  ")
		if err != nil {
			return err
		}
		_, err = cmd.Root().Writer.Write(append(data, '\n'))
		return err
	}}
}
