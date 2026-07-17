package skillinstall

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestTargetsResolveExplicitClients(t *testing.T) {
	userHome := t.TempDir()
	tests := []struct {
		client string
		path   string
	}{
		{client: ClientCodex, path: filepath.Join(userHome, ".agents", "skills", "nore")},
		{client: ClientClaude, path: filepath.Join(userHome, ".claude", "skills", "nore")},
		{client: ClientCursor, path: filepath.Join(userHome, ".cursor", "skills", "nore")},
	}
	for _, test := range tests {
		t.Run(test.client, func(t *testing.T) {
			targets, err := Targets(userHome, test.client)
			if err != nil {
				t.Fatalf("resolve target: %v", err)
			}
			if len(targets) != 1 || targets[0].Path != test.path {
				t.Fatalf("unexpected targets: %#v", targets)
			}
		})
	}
}

func TestTargetsAutoDetectInstalledClients(t *testing.T) {
	userHome := t.TempDir()
	for _, directory := range []string{".codex", ".claude"} {
		if err := os.MkdirAll(filepath.Join(userHome, directory), 0o755); err != nil {
			t.Fatalf("create client directory: %v", err)
		}
	}

	targets, err := Targets(userHome, ClientAuto)
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 2 || targets[0].Client != ClientCodex || targets[1].Client != ClientClaude {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestTargetsRejectUnsupportedClient(t *testing.T) {
	if _, err := Targets(t.TempDir(), "other"); !errors.Is(err, ErrUnsupportedClient) {
		t.Fatalf("expected unsupported client error, got: %v", err)
	}
}

func TestTargetsRequireDetectedClientForAuto(t *testing.T) {
	if _, err := Targets(t.TempDir(), ClientAuto); !errors.Is(err, ErrNoClientDetected) {
		t.Fatalf("expected no detected client error, got: %v", err)
	}
}

func TestInstallReplacesExistingSkillDirectory(t *testing.T) {
	target := Target{
		Client:      ClientCodex,
		DisplayName: "Codex",
		Path:        filepath.Join(t.TempDir(), ".agents", "skills", "nore"),
	}
	if err := os.MkdirAll(target.Path, 0o755); err != nil {
		t.Fatalf("create existing skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "stale.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	bundle := fstest.MapFS{
		"SKILL.md":            {Data: []byte("---\nname: nore\n---\n")},
		"references/usage.md": {Data: []byte("usage\n")},
	}

	if err := Install(bundle, target); err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target.Path, "stale.md")); err == nil {
		t.Fatal("stale file must be removed")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect stale file: %v", err)
	}
	for _, path := range []string{
		filepath.Join(target.Path, "SKILL.md"),
		filepath.Join(target.Path, "references", "usage.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("installed skill is incomplete: %s: %v", path, err)
		}
	}
}
