package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThalesGroup/helm-spray/v4/cmd"
)

func main() {
	// Cancel the run on SIGINT/SIGTERM so in-flight helm/kubectl child processes
	// are terminated cleanly via the propagated context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := cmd.NewRootCmd()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
