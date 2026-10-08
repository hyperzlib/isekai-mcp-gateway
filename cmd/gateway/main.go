package main

import (
	"context"
	"fmt"
	gatewaycli "isekai-mcp-gateway/internal/cli"
	"os"
)

func main() {
	if err := gatewaycli.Run(context.Background(), os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
