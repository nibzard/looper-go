package agents

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/nibzard/looper-go/internal/utils"
)

// NewAgent creates an agent of the specified type.
// It uses the agent registry to find the appropriate factory.
// If the agent type is not registered, it falls back to a generic agent.
func NewAgent(agentType AgentType, cfg Config) (Agent, error) {
	factory, ok := Registry[agentType]
	if !ok {
		// Fall back to generic agent for unknown types
		return NewGenericAgent(string(agentType), cfg)
	}
	return factory(cfg)
}

// FindAgentBinary finds the agent binary in PATH.
// For built-in agents (codex, claude), it uses the default binary names.
// For custom agents, it uses the agent type name as the binary name.
func FindAgentBinary(agentType AgentType) (string, error) {
	var name string
	switch agentType {
	case AgentTypeCodex:
		name = "codex"
	case AgentTypeClaude:
		name = "claude"
	default:
		// For custom agent types, use the agent type name as the binary name
		name = string(agentType)
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("agent binary %q not found in PATH: %w", name, err)
	}
	return path, nil
}

// ValidateBinary checks if a binary exists and is executable.
// On Windows, we only check if the file exists and has a valid executable extension.
// On Unix, we also check the execute permission bit.
func ValidateBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("binary not found: %s", path)
		}
		return fmt.Errorf("stat binary: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("binary path is a directory: %s", path)
	}

	if runtime.GOOS == "windows" {
		if !isWindowsExecutable(path) {
			return fmt.Errorf("binary is not executable: %s", path)
		}
		return nil
	}
	if info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary is not executable: %s", path)
	}

	return nil
}

func isWindowsExecutable(path string) bool {
	return utils.IsWindowsExecutable(path)
}

// CreateLogWriter creates a log writer from the given writer. If LOOPER_QUIET
// is not set, it also creates a stdout writer and returns a MultiLogWriter
// that writes to both the primary writer and stdout with indentation.
func CreateLogWriter(writer io.Writer) LogWriter {
	logWriter := NewIOStreamLogWriter(writer)
	if os.Getenv("LOOPER_QUIET") != "" {
		return logWriter
	}
	stdoutWriter := NewIOStreamLogWriter(os.Stdout)
	stdoutWriter.SetIndent("  ")
	return NewMultiLogWriter(logWriter, stdoutWriter)
}
