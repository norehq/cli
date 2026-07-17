package skillinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	ClientAll    = "all"
	ClientAuto   = "auto"
	ClientClaude = "claude"
	ClientCodex  = "codex"
	ClientCursor = "cursor"
)

var (
	ErrNoClientDetected  = errors.New("no supported client detected")
	ErrUnsupportedClient = errors.New("unsupported skill client")
)

type Target struct {
	Client      string `json:"client"`
	DisplayName string `json:"displayName"`
	Path        string `json:"path"`
}

type clientSpec struct {
	client             string
	detectionDirectory string
	displayName        string
	skillDirectory     string
}

var clientSpecs = []clientSpec{
	{
		client:             ClientCodex,
		detectionDirectory: ".codex",
		displayName:        "Codex",
		skillDirectory:     filepath.Join(".agents", "skills", "nore"),
	},
	{
		client:             ClientClaude,
		detectionDirectory: ".claude",
		displayName:        "Claude Code",
		skillDirectory:     filepath.Join(".claude", "skills", "nore"),
	},
	{
		client:             ClientCursor,
		detectionDirectory: ".cursor",
		displayName:        "Cursor",
		skillDirectory:     filepath.Join(".cursor", "skills", "nore"),
	},
}

func SupportedClients() []string {
	return []string{ClientAuto, ClientCodex, ClientClaude, ClientCursor, ClientAll}
}

func Targets(userHome string, requested string) ([]Target, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "" {
		requested = ClientAuto
	}
	if requested == ClientAll {
		return targetsForSpecs(userHome, clientSpecs), nil
	}
	if requested != ClientAuto {
		for _, spec := range clientSpecs {
			if spec.client == requested {
				return targetsForSpecs(userHome, []clientSpec{spec}), nil
			}
		}
		return nil, ErrUnsupportedClient
	}

	detected := make([]clientSpec, 0, len(clientSpecs))
	for _, spec := range clientSpecs {
		path := filepath.Join(userHome, spec.detectionDirectory)
		if _, err := os.Stat(path); err == nil {
			detected = append(detected, spec)
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if len(detected) == 0 {
		return nil, ErrNoClientDetected
	}
	return targetsForSpecs(userHome, detected), nil
}

func Install(bundle fs.FS, target Target) error {
	parent := filepath.Dir(target.Path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".nore-skill-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(temporary)
	}()
	if err := os.Chmod(temporary, 0o755); err != nil {
		return err
	}
	if err := copyBundle(bundle, temporary); err != nil {
		return err
	}
	if err := os.RemoveAll(target.Path); err != nil {
		return err
	}
	return os.Rename(temporary, target.Path)
}

func targetsForSpecs(userHome string, specs []clientSpec) []Target {
	targets := make([]Target, 0, len(specs))
	for _, spec := range specs {
		targets = append(targets, Target{
			Client:      spec.client,
			DisplayName: spec.displayName,
			Path:        filepath.Join(userHome, spec.skillDirectory),
		})
	}
	return targets
}

func copyBundle(bundle fs.FS, destination string) error {
	return fs.WalkDir(bundle, ".", func(
		path string,
		entry fs.DirEntry,
		walkErr error,
	) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		payload, err := fs.ReadFile(bundle, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o644)
	})
}
