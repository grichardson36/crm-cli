package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jdanielnd/crm-cli/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cli.Version = version
	os.Exit(run())
}

func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rootCmd := cli.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		return cli.ReportError(os.Stderr, err, os.Args[1:])
	}
	return 0
}

// Used by GoReleaser for version info.
var _ = commit
