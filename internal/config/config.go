package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func DefaultDataDir() (string, error) {
	if value := os.Getenv("QUIZDOCK_DATA_DIR"); value != "" {
		return filepath.Abs(value)
	}
	var root string
	var err error
	switch runtime.GOOS {
	case "windows":
		root = os.Getenv("LOCALAPPDATA")
		if root == "" {
			root, err = os.UserConfigDir()
		}
	case "darwin":
		root, err = os.UserHomeDir()
		root = filepath.Join(root, "Library", "Application Support")
	default:
		root = os.Getenv("XDG_DATA_HOME")
		if root == "" {
			root, err = os.UserHomeDir()
			root = filepath.Join(root, ".local", "share")
		}
	}
	if err != nil || root == "" {
		return "", fmt.Errorf("resolve user data directory: %w", err)
	}
	return filepath.Join(root, "quizdock"), nil
}
