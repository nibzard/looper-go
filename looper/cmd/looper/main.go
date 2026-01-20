// Command looper is the CLI entrypoint for the Go rewrite.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nibzard/looper/internal/config"
)

func main() {
	// Define a simple flag set for the CLI
	fs := flag.NewFlagSet("looper", flag.ExitOnError)
	cfg, err := config.Load(fs, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// For now, just print the loaded config to demonstrate it works
	fmt.Printf("Config loaded successfully:\n")
	fmt.Printf("  TodoFile: %s\n", cfg.TodoFile)
	fmt.Printf("  SchemaFile: %s\n", cfg.SchemaFile)
	fmt.Printf("  LogDir: %s\n", cfg.LogDir)
	fmt.Printf("  MaxIterations: %d\n", cfg.MaxIterations)
	fmt.Printf("  Schedule: %s\n", cfg.Schedule)
	fmt.Printf("  ApplySummary: %v\n", cfg.ApplySummary)
	fmt.Printf("  ProjectRoot: %s\n", cfg.ProjectRoot)
}
