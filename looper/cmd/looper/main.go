// Command looper is the CLI entrypoint for the Go rewrite.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nibzard/looper/internal/config"
	"github.com/nibzard/looper/internal/loop"
)

func main() {
	// Define a simple flag set for the CLI
	fs := flag.NewFlagSet("looper", flag.ExitOnError)
	cfg, err := config.Load(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

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

	// Create and run loop
	l, err := loop.New(cfg, cfg.ProjectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing loop: %v\n", err)
		os.Exit(1)
	}

	if err := l.Run(ctx); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "\nLoop interrupted\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error running loop: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("Loop completed")
}
