package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/nibzard/looper-go/internal/config"
)

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	runErr := fn()
	_ = w.Close()

	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("ReadAll() error = %v", readErr)
	}

	return string(output), runErr
}

func TestCompletionCommandOutputsScripts(t *testing.T) {
	cfg := &config.Config{}

	tests := []struct {
		name    string
		shell   string
		needle  string
		wantErr bool
	}{
		{
			name:   "bash",
			shell:  "bash",
			needle: "# looper bash completion",
		},
		{
			name:   "zsh",
			shell:  "zsh",
			needle: "#compdef looper",
		},
		{
			name:   "fish",
			shell:  "fish",
			needle: "# looper fish completion",
		},
		{
			name:   "powershell",
			shell:  "powershell",
			needle: "# looper PowerShell completion",
		},
		{
			name:   "pwsh alias",
			shell:  "pwsh",
			needle: "# looper PowerShell completion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := captureStdout(t, func() error {
				return completionCommand(cfg, []string{tt.shell})
			})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for shell %q, got nil", tt.shell)
				}
				return
			}
			if err != nil {
				t.Fatalf("completionCommand() error = %v", err)
			}
			if !strings.Contains(output, tt.needle) {
				t.Fatalf("completion output missing %q for shell %q", tt.needle, tt.shell)
			}
		})
	}
}

func TestCompletionCommandErrors(t *testing.T) {
	cfg := &config.Config{}

	if err := completionCommand(cfg, []string{}); err == nil {
		t.Fatal("expected error when shell is missing")
	}

	if err := completionCommand(cfg, []string{"unknown"}); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}
