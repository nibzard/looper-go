# Makefile for Looper Amp Plugin

.PHONY: all build clean install validate test help

BINARY_NAME=amp
BINARY_PATH=./bin/$(BINARY_NAME)
SOURCE_FILE=main.go

# Go build flags
GO=go
GOFLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"

# Default target
all: build

# Build the plugin binary
build:
	@echo "Building $(BINARY_NAME) plugin..."
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_PATH) $(SOURCE_FILE)
	@chmod +x $(BINARY_PATH)
	@echo "Built: $(BINARY_PATH)"

# Install the plugin to user directory
install: build
	@echo "Installing plugin to ~/.looper/plugins/amp..."
	@mkdir -p ~/.looper/plugins/amp
	@cp looper-plugin.toml ~/.looper/plugins/amp/
	@cp $(BINARY_PATH) ~/.looper/plugins/amp/bin/
	@echo "Plugin installed successfully"

# Validate the plugin
validate:
	@echo "Validating plugin..."
	@if command -v looper >/dev/null 2>&1; then \
		looper plugin validate .; \
	else \
		echo "Looper not found in PATH. Skipping validation."; \
	fi

# Test the plugin
test: build
	@echo "Testing plugin info method..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"info"}' | $(BINARY_PATH)
	@echo ""
	@echo "Testing plugin run method..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"run","params":{"prompt":"What is 2+2? Simply reply with the number."}}' | $(BINARY_PATH)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin
	@echo "Cleaned"

# Development build (with debug info)
dev:
	@echo "Building $(BINARY_NAME) plugin (debug)..."
	@mkdir -p bin
	$(GO) build -o $(BINARY_PATH) $(SOURCE_FILE)
	@chmod +x $(BINARY_PATH)
	@echo "Built: $(BINARY_PATH) (debug)"

# Show help
help:
	@echo "Looper Amp Plugin - Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  all       - Build the plugin (default)"
	@echo "  build     - Build the plugin binary"
	@echo "  install   - Install to ~/.looper/plugins/"
	@echo "  validate  - Validate plugin manifest"
	@echo "  test      - Run basic tests"
	@echo "  clean     - Remove build artifacts"
	@echo "  dev       - Build with debug info"
	@echo "  help      - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make              # Build the plugin"
	@echo "  make install      # Build and install"
	@echo "  make test         # Test the plugin"
