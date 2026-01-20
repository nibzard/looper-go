// Package config handles configuration loading and defaults.
//
// Configuration is loaded from multiple sources in priority order:
// 1. Built-in defaults
// 2. Config file (looper.toml) in the project root
// 3. Environment variables (LOOPER_*)
// 4. CLI flags
//
// Each level overrides the previous one, so CLI flags take precedence.
package config
