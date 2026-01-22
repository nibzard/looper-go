package utils

import (
	"os"
	"testing"
)

func TestWindowsExecutableExtensions(t *testing.T) {
	// Save original PATHEXT
	origPathext := os.Getenv("PATHEXT")
	defer func() {
		if origPathext == "" {
			os.Unsetenv("PATHEXT")
		} else {
			os.Setenv("PATHEXT", origPathext)
		}
	}()

	tests := []struct {
		name     string
		pathext  string
		wantExts map[string]bool
	}{
		{
			name:    "default PATHEXT",
			pathext: "",
			wantExts: map[string]bool{
				".com": true,
				".exe": true,
				".bat": true,
				".cmd": true,
			},
		},
		{
			name:    "custom PATHEXT",
			pathext: ".COM;.EXE;.PS1",
			wantExts: map[string]bool{
				".com": true,
				".exe": true,
				".ps1": true,
			},
		},
		{
			name:    "PATHEXT without dots",
			pathext: "COM;EXE;BAT",
			wantExts: map[string]bool{
				".com": true,
				".exe": true,
				".bat": true,
			},
		},
		{
			name:    "mixed format with spaces",
			pathext: ".COM; .EXE ; .BAT",
			wantExts: map[string]bool{
				".com": true,
				".exe": true,
				".bat": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pathext == "" {
				os.Unsetenv("PATHEXT")
			} else {
				os.Setenv("PATHEXT", tt.pathext)
			}

			got := WindowsExecutableExtensions()
			for ext := range tt.wantExts {
				if !got[ext] {
					t.Errorf("WindowsExecutableExtensions() missing extension %q", ext)
				}
			}
		})
	}
}

func TestIsWindowsExecutable(t *testing.T) {
	// Set a known PATHEXT
	origPathext := os.Getenv("PATHEXT")
	defer func() {
		if origPathext == "" {
			os.Unsetenv("PATHEXT")
		} else {
			os.Setenv("PATHEXT", origPathext)
		}
	}()
	os.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exe file", "C:\\Program Files\\app\\executable.exe", true},
		{"bat file", "C:\\script.bat", true},
		{"cmd file", "C:\\command.cmd", true},
		{"com file", "C:\\program.com", true},
		{"uppercase extension", "C:\\app.EXE", true},
		{"no extension", "C:\\app", false},
		{"text file", "C:\\readme.txt", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWindowsExecutable(tt.path); got != tt.want {
				t.Errorf("IsWindowsExecutable() = %v, want %v", got, tt.want)
			}
		})
	}
}
