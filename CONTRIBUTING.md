# Contributing to Looper-Go

Thank you for your interest in contributing to Looper-Go! This project is an AI agent orchestration tool, and we welcome contributions from developers of all experience levels—including those using AI assistance.

## Quick Start

1. Fork the repository
2. Create a branch for your work
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

## Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/looper-go.git
cd looper-go

# Install dependencies
go mod download

# Run tests
make test

# Build
go build ./cmd/looper
```

## Code Style

- Follow standard Go conventions (`gofmt`)
- Write tests for new functionality
- Update documentation as needed
- Keep changes focused and minimal

## Pre-commit Hooks

We use pre-commit to automatically check code quality before commits.

### Installation

```bash
# Install pre-commit (if not already installed)
pip install pre-commit

# Or with brew on macOS
brew install pre-commit

# Install the hooks
pre-commit install
```

### What Gets Checked

- **Formatting**: `gofmt` ensures consistent code style
- **Linting**: `golangci-lint` catches common issues
- **Vet**: `go vet` finds suspicious constructs
- **Module tidy**: Ensures `go.mod` is clean
- **File hygiene**: Trailing whitespace, line endings, large files

### Bypassing Hooks

If you need to skip hooks (not recommended):
```bash
git commit --no-verify -m "..."
```

### Running Hooks Manually

```bash
# Run on all files
pre-commit run --all-files

# Run on specific files
pre-commit run --files path/to/file.go
```

## AI-Assisted Contributions

**We welcome AI-assisted contributions!** Looper-Go is a tool for AI agent orchestration, and we believe AI can be a valuable partner in software development.

### Requirements for AI-Assisted PRs

1. **Disclose AI usage** in your pull request description
2. **Test thoroughly** - AI can make mistakes, so please verify functionality
3. **Understand the code** - be prepared to explain and maintain your contribution
4. **Use AI responsibly** - don't paste code you don't understand

### Example Disclosure

> This PR was written with assistance from Claude Code. I reviewed all changes and verified tests pass.

### What We Encourage

- Using AI for code generation, refactoring, and documentation
- Learning from AI explanations and suggestions
- Experimenting with AI agents to explore the codebase

### What We Discourage

- Submitting untested AI-generated code
- Using AI without understanding the changes
- Relying solely on AI for complex architectural decisions

## Pull Request Process

1. Ensure your code passes all tests
2. Update documentation if needed
3. Add a clear description of your changes
4. Reference related issues
5. Wait for code review feedback

## Reporting Issues

When reporting bugs, please include:

- Looper-Go version
- Go version
- Operating system
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

## Feature Requests

We welcome feature requests! Please:

- Check existing issues first
- Describe the use case clearly
- Consider if it fits the project's scope
- Be open to discussion

## Code of Conduct

Please be respectful and constructive. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for details.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
