package config

import "os"

// devModeEnabled returns true if dev mode is enabled via LOOPER_PROMPT_MODE=dev.
// Dev mode enables --prompt-dir and --print-prompt flags for prompt development.
func devModeEnabled() bool {
	return os.Getenv("LOOPER_PROMPT_MODE") == "dev"
}

// PromptDevModeEnabled reports whether prompt development options are enabled.
func PromptDevModeEnabled() bool {
	return devModeEnabled()
}
