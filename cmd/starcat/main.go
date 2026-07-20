// starcat 是跨平台 Starcat CLI，同时可作为外部 AI Agent 的 stdio MCP Server。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/starcat-app/starcat-cli/internal/cli"
	"github.com/starcat-app/starcat-cli/internal/mcp"
	"github.com/starcat-app/starcat-cli/internal/updater"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runner, err := cli.NewRunner(os.Stdin, os.Stdout, os.Stderr)
	if err == nil {
		err = runner.Run(ctx, os.Args[1:])
	}
	if err == nil {
		updater.MaybeNotify(ctx, mcp.Version, os.Args[1:], os.Stderr)
		return
	}
	if errors.Is(err, context.Canceled) {
		return
	}
	// stdout 是 JSON/MCP 协议通道，所有错误只能写 stderr。
	fmt.Fprintln(os.Stderr, "starcat:", err)
	os.Exit(1)
}
