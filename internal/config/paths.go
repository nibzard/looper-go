package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// expandPath expands home directory and environment variables in paths.
// It supports ~/ or ~\ prefixes and %VAR% expansion on Windows.
func expandPath(p string) string {
	if p == "" {
		return p
	}

	expanded := expandEnv(p)
	if expanded == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return expanded
		}
		return home
	}
	if strings.HasPrefix(expanded, "~/") || (runtime.GOOS == "windows" && strings.HasPrefix(expanded, "~\\")) {
		home, err := os.UserHomeDir()
		if err != nil {
			return expanded
		}
		return filepath.Join(home, expanded[2:])
	}
	return expanded
}

func expandEnv(p string) string {
	expanded := os.ExpandEnv(p)
	if runtime.GOOS != "windows" {
		return expanded
	}
	return expandWindowsEnv(expanded)
}

func expandWindowsEnv(p string) string {
	if !strings.Contains(p, "%") {
		return p
	}
	var b strings.Builder
	for i := 0; i < len(p); {
		if p[i] == '%' {
			end := strings.IndexByte(p[i+1:], '%')
			if end >= 0 {
				key := p[i+1 : i+1+end]
				if key == "" {
					b.WriteByte('%')
					i++
					continue
				}
				if val, ok := os.LookupEnv(key); ok {
					b.WriteString(val)
				} else {
					b.WriteByte('%')
					b.WriteString(key)
					b.WriteByte('%')
				}
				i += end + 2
				continue
			}
		}
		b.WriteByte(p[i])
		i++
	}
	return b.String()
}
