package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/benoitdion/buienradarcli/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	os.Exit(cli.Execute(ctx))
}
