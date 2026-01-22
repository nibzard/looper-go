// Package utils provides shared utility functions used across multiple packages.
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// WindowsExecutableExtensions returns a map of lowercase Windows executable
// extensions (with leading dot) to true, parsed from the PATHEXT environment
// variable. Returns a default set if PATHEXT is unset.
func WindowsExecutableExtensions() map[string]bool {
	exts := map[string]bool{}
	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	for _, ext := range strings.Split(pathext, ";") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		exts[strings.ToLower(ext)] = true
	}
	return exts
}

// IsWindowsExecutable returns true if the given file path has a Windows
// executable extension according to the PATHEXT environment variable.
func IsWindowsExecutable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	return WindowsExecutableExtensions()[ext]
}
