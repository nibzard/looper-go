// Package utils provides shared utility functions used across multiple packages.
package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// SplitAndTrim splits a string by sep and trims whitespace from each part.
// Empty parts are omitted from the result.
func SplitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// JSONPointerToPath converts a JSON Pointer (RFC 6901) to a dot-notation path.
// For example, "#/foo/bar/0/baz" becomes "foo.bar[0].baz".
// This is useful for converting JSON Schema validation error locations to
// human-readable paths.
func JSONPointerToPath(ptr string) string {
	if ptr == "" {
		return ""
	}
	if strings.HasPrefix(ptr, "#") {
		ptr = strings.TrimPrefix(ptr, "#")
	}
	if strings.HasPrefix(ptr, "/") {
		ptr = ptr[1:]
	}
	if ptr == "" {
		return ""
	}

	parts := strings.Split(ptr, "/")
	path := ""
	for _, part := range parts {
		// Unescape JSON Pointer reserved characters
		// ~1 represents /
		// ~0 represents ~
		part = strings.ReplaceAll(part, "~1", "/")
		part = strings.ReplaceAll(part, "~0", "~")
		if part == "" {
			continue
		}
		// Array indices are represented with brackets
		if idx, err := strconv.Atoi(part); err == nil {
			path += fmt.Sprintf("[%d]", idx)
			continue
		}
		if path == "" {
			path = part
		} else {
			path += "." + part
		}
	}

	return path
}
