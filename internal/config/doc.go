// Package config handles configuration loading and defaults.
//
// Configuration is loaded from multiple sources in priority order:
// 1. Built-in defaults
// 2. User config file (~/.looper/looper.toml or OS-specific config directory)
// 3. Project config file (looper.toml or .looper.toml in the project root)
// 4. Environment variables (LOOPER_*)
// 5. CLI flags
//
// Each level overrides the previous one, so CLI flags take precedence.
//
// User-level config locations:
// - ~/.looper/looper.toml (preferred)
// - Windows: %APPDATA%\looper\looper.toml
// - macOS: ~/Library/Application Support/looper/looper.toml
// - Linux/BSD: $XDG_CONFIG_HOME/looper/looper.toml or ~/.config/looper/looper.toml
//
// Project-level config locations (overrides user config):
// - ./looper.toml (preferred)
// - ./.looper.toml
package config
