package command

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bundledskill "github.com/norehq/cli/skill"
)

func TestSkillUpdateInstallsBundledSkillForCodex(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute(
		[]string{"skill", "update", "--client", "codex", "--json"},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must be empty in JSON mode: %q", stderr.String())
	}

	var result updateSkillData
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if result.SkillVersion == "" || len(result.Targets) != 1 || result.Targets[0].Client != "codex" {
		t.Fatalf("unexpected result: %#v", result)
	}
	path := filepath.Join(userHome, ".agents", "skills", "nore", "SKILL.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("installed skill is missing SKILL.md: %v", err)
	}
}

func TestSkillUpdateRejectsUnsupportedClient(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute(
		[]string{"skill", "update", "--client", "other", "--json"},
		&stdout,
		&stderr,
	)
	if exitCode == 0 {
		t.Fatal("expected a non-zero exit code")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must be empty for JSON errors: %q", stdout.String())
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &result); err != nil {
		t.Fatalf("stderr is not one JSON document: %v", err)
	}
	if result.Error.Code != "UNSUPPORTED_CLIENT" {
		t.Fatalf("unexpected error: %#v", result.Error)
	}
}

func TestSkillUpdateSupportsAllClients(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute(
		[]string{"skill", "update", "--client", "all", "--json"},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", exitCode, stderr.String())
	}

	var result updateSkillData
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if len(result.Targets) != 3 {
		t.Fatalf("unexpected targets: %#v", result.Targets)
	}
	for _, target := range result.Targets {
		if _, err := os.Stat(filepath.Join(target.Path, "SKILL.md")); err != nil {
			t.Fatalf("%s skill was not created: %v", target.Client, err)
		}
	}
}

func TestSkillShowPrintsBundledSkillText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute(
		[]string{"skill", "show", "--no-color"},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must be empty: %q", stderr.String())
	}
	text, err := bundledskill.Text()
	if err != nil {
		t.Fatalf("read bundled skill text: %v", err)
	}
	if stdout.String() != text {
		t.Fatalf("unexpected skill text:\n%s", stdout.String())
	}
}

func TestSkillShowJSONIncludesVersionAndText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Execute(
		[]string{"skill", "show", "--json"},
		&stdout,
		&stderr,
	)
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\n%s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must be empty in JSON mode: %q", stderr.String())
	}

	var result showSkillData
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if result.SkillVersion == "" || !strings.Contains(result.Text, "name: nore") {
		t.Fatalf("unexpected result: %#v", result)
	}
}
