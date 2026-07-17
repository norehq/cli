package home

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const environment = "NORE_HOME"

var ErrRelativePath = errors.New("NORE_HOME must be an absolute path")

func Directory() (string, error) {
	if directory := strings.TrimSpace(os.Getenv(environment)); directory != "" {
		if !filepath.IsAbs(directory) {
			return "", fmt.Errorf("%w: %s", ErrRelativePath, directory)
		}
		return filepath.Clean(directory), nil
	}
	directory, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, ".nore"), nil
}

func Path(name string) (string, error) {
	directory, err := Directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}

func DisplayPath(path string) string {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return path
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(userHome, path)
	if err != nil {
		return path
	}
	if relative == "." {
		return "~"
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join("~", relative)
}
