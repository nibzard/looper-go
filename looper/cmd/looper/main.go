// Command looper is the CLI entrypoint for the Go rewrite.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nibzard/looper/cmd"
)

func main() {
	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Run the CLI
	if err := cmd.Run(ctx, os.Args[1:]); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "\nInterrupted\n")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
