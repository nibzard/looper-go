package parsers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// newPythonParser creates a parser that executes a Python script.
func newPythonParser(scriptPath string) (Parser, error) {
	// Resolve the script path
	if !filepath.IsAbs(scriptPath) {
		abs, err := filepath.Abs(scriptPath)
		if err != nil {
			return nil, fmt.Errorf("resolve script path: %w", err)
		}
		scriptPath = abs
	}

	// Check if script exists
	if info, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("script not found: %s: %w", scriptPath, err)
	} else if info.IsDir() {
		return nil, fmt.Errorf("script path is a directory: %s", scriptPath)
	}

	return &pythonParser{script: scriptPath}, nil
}

type pythonParser struct {
	script string
}

func (p *pythonParser) Parse(ctx context.Context, output string) (*Summary, error) {
	// Find Python interpreter
	python, err := findPython()
	if err != nil {
		return nil, fmt.Errorf("find python: %w", err)
	}

	// Execute the parser script
	cmd := exec.CommandContext(ctx, python, p.script)
	cmd.Stdin = strings.NewReader(output)
	cmd.Stderr = os.Stderr // Let stderr show through for debugging

	rawOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run parser script: %w", err)
	}

	// Parse the JSON output from the script
	return ParseJSONSummary(string(rawOutput))
}

// newJSParser creates a parser that executes a JavaScript script.
func newJSParser(scriptPath string) (Parser, error) {
	// Resolve the script path
	if !filepath.IsAbs(scriptPath) {
		abs, err := filepath.Abs(scriptPath)
		if err != nil {
			return nil, fmt.Errorf("resolve script path: %w", err)
		}
		scriptPath = abs
	}

	// Check if script exists
	if info, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("script not found: %s: %w", scriptPath, err)
	} else if info.IsDir() {
		return nil, fmt.Errorf("script path is a directory: %s", scriptPath)
	}

	return &jsParser{script: scriptPath}, nil
}

type jsParser struct {
	script string
}

func (p *jsParser) Parse(ctx context.Context, output string) (*Summary, error) {
	// Find Node.js interpreter
	node, err := findNode()
	if err != nil {
		return nil, fmt.Errorf("find node: %w", err)
	}

	// Execute the parser script
	cmd := exec.CommandContext(ctx, node, p.script)
	cmd.Stdin = strings.NewReader(output)
	cmd.Stderr = os.Stderr // Let stderr show through for debugging

	rawOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run parser script: %w", err)
	}

	// Parse the JSON output from the script
	return ParseJSONSummary(string(rawOutput))
}

// findPython finds the Python interpreter.
// Checks common Python command names.
func findPython() (string, error) {
	// Try common Python commands in order of preference
	pythonCmds := []string{"python3", "python"}

	for _, cmd := range pythonCmds {
		if path, err := exec.LookPath(cmd); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("python interpreter not found in PATH (tried: %s)", strings.Join(pythonCmds, ", "))
}

// findNode finds the Node.js interpreter.
// Checks common Node.js command names.
func findNode() (string, error) {
	// Try common Node commands in order of preference
	nodeCmds := []string{"node", "nodejs"}

	for _, cmd := range nodeCmds {
		if path, err := exec.LookPath(cmd); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("node.js interpreter not found in PATH (tried: %s)", strings.Join(nodeCmds, ", "))
}

// ValidateParserScript checks if a parser script is valid.
// It verifies the file exists, is executable, and has a supported extension.
func ValidateParserScript(path string) error {
	if path == "" {
		return fmt.Errorf("parser script path is empty")
	}

	// Expand ~ if present
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("expand home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("parser script not found: %s", path)
		}
		return fmt.Errorf("stat parser script: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("parser script is a directory: %s", path)
	}

	// Check extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".py", ".js":
		// Valid extension
	default:
		return fmt.Errorf("unsupported parser extension: %s (supported: .py, .js)", ext)
	}

	// On Unix, check if file is executable
	if runtime.GOOS != "windows" {
		if info.Mode().Perm()&0111 == 0 {
			return fmt.Errorf("parser script is not executable: %s (run: chmod +x %s)", path, path)
		}
	}

	return nil
}
