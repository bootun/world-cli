package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/bootun/world-cli/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	cancel()
	os.Exit(code)
}
