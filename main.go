package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThalesGroup/helm-spray/v4/cmd"
)

func main() {
	os.Exit(run())
}

// run wires up signal cancellation and executes the root command, returning the
// process exit code. Keeping os.Exit out of this function's scope ensures the
// deferred stop() actually runs before the process terminates.
func run() int {
	// Cancel the run on SIGINT/SIGTERM so any in-flight helm child process is
	// terminated cleanly via the propagated context.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.NewRootCmd().ExecuteContext(ctx); err != nil {
		return 1
	}
	return 0
}
