# Looper Amp Plugin

A [Looper](https://github.com/nibzard/looper-go) plugin for [Amp Code](https://ampcode.com) - the frontier coding agent with multi-model support.

## Features

- **Multi-model support**: Amp automatically selects the best model (Opus 4.5, GPT-5.2, Gemini 3 Pro, Sonnet 4.5) based on your task and mode
- **Modes**: Smart (state-of-the-art), Rush (faster/cheaper), Large (more context)
- **Streaming output**: Real-time response streaming via JSON-RPC
- **Tool support**: Full tool use including file editing, shell commands
- **MCP support**: Model Context Protocol for extending capabilities
- **Non-interactive mode**: Uses `--dangerously-allow-all` for automated workflows

## Installation

### Via Looper Plugin CLI

```bash
# Install from this directory
looper plugin install ./plugins/amp

# Or from a git repository
looper plugin install https://github.com/nibzard/looper-amp-plugin
```

### Manual Installation

```bash
# Copy to user plugins directory
cp -r plugins/amp ~/.looper/plugins/amp

# Or project-specific
cp -r plugins/amp .looper/plugins/amp
```

### Build from Source

```bash
cd plugins/amp
make build
```

## Usage

### Configuration

Add to your `looper.toml`:

```toml
[roles]
iter = "amp"
review = "amp"

[agents.amp]
timeout = "30m"
```

### Command Line

```bash
# Run with Amp agent
looper run --schedule amp

# Pass extra arguments to Amp
looper run --amp-args "--timeout,60m"
```

## Amp Modes (Not Configurable via Looper)

Amp uses **modes** instead of direct model selection:

| Mode | Description |
|------|-------------|
| `smart` | State-of-the-art models (Opus 4.5) - default |
| `rush` | Faster, cheaper models for small tasks |
| `large` | Hidden mode for larger context |

**Mode switching** is done via:
- Amp CLI: Press `Ctrl+O`, type "mode"
- Editor extension: Select mode in prompt field

**Note**: Looper cannot control Amp's mode selection. Configure your preferred mode in Amp's settings (`~/.config/amp/settings.json`) or via the Amp command palette before running Looper.

## Amp Non-Interactive Mode

This plugin uses Amp's `--dangerously-allow-all` flag for non-interactive operation. This is equivalent to:

| Tool | Permission Bypass Flag |
|------|------------------------|
| **Amp** | `--dangerously-allow-all` |
| **Claude Code** | `--dangerously-skip-permissions` |
| **Codex** | `--dangerously-bypass-approvals-and-sandbox` |

## Plugin Protocol

This plugin implements the Looper JSON-RPC agent protocol:

### Request

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "run",
  "params": {
    "prompt": "Your task here",
    "context": {
      "model": "opus-4-5"
    }
  }
}
```

### Response

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "task_id": "T-001",
    "status": "done",
    "summary": "Task completed successfully",
    "files": ["file1.go", "file2.go"],
    "blockers": []
  }
}
```

### Streaming

```json
{"type":"system","subtype":"init",...}
{"type":"user",...}
{"type":"assistant",...}
{"type":"result","subtype":"success",...}
```

## Amp Binary Requirements

This plugin requires the `amp` binary to be installed and available in your PATH.

Install Amp:
```bash
curl -fsSL https://ampcode.com/install.sh | bash
```

Or see https://ampcode.com for installation instructions.

### Amp Configuration

Amp's model selection is configured through:
- **Settings file**: `~/.config/amp/settings.json`
- **Command palette**: Press `Ctrl+O` in CLI, then type "mode"

See [Amp Owner's Manual](https://ampcode.com/manual) for more details.

## License

MIT

## Related

- [Looper Documentation](https://github.com/nibzard/looper-go)
- [Amp Code Documentation](https://ampcode.com/manual)
- [Looper Plugin Development Guide](https://github.com/nibzard/looper-go/blob/main/ARCHITECTURE.md#plugin-system)
