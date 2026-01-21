// Package hooks invokes external post-iteration hooks.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Options configures a hook invocation.
type Options struct {
	Command         string
	LastMessagePath string
	Label           string
	WorkDir         string
}

// Result captures the outcome of a hook invocation.
type Result struct {
	Ran      bool
	Command  []string
	ExitCode int
	TaskID   string
	Status   string
}

// Invoke runs the hook command with the expected arguments.
func Invoke(ctx context.Context, opts Options) (Result, error) {
	if opts.Command == "" || opts.LastMessagePath == "" {
		return Result{}, nil
	}

	info, err := os.Stat(opts.LastMessagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, nil
		}
		return Result{}, fmt.Errorf("stat last message file: %w", err)
	}
	if info.IsDir() {
		return Result{}, fmt.Errorf("last message path is a directory: %s", opts.LastMessagePath)
	}

	raw, err := readLastMessage(opts.LastMessagePath)
	if err != nil {
		return Result{}, err
	}

	taskID, status := extractSummaryFields(raw)
	args := []string{taskID, status, opts.LastMessagePath, opts.Label}

	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, opts.Command, args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	exitCode := exitCodeFromError(err)
	result := Result{
		Ran:      true,
		Command:  cmd.Args,
		ExitCode: exitCode,
		TaskID:   taskID,
		Status:   status,
	}
	if err != nil {
		return result, fmt.Errorf("hook command failed: %w", err)
	}
	return result, nil
}

func readLastMessage(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read last message: %w", err)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("last message is empty: %s", path)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("last message is not valid JSON: %s", path)
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse last message: %w", err)
	}
	return raw, nil
}

func extractSummaryFields(raw any) (string, string) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return "", ""
	}
	return stringField(obj["task_id"]), stringField(obj["status"])
}

func stringField(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
