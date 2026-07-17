package output

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestHelpUsesColorWhenForced(t *testing.T) {
	restoreEnvironment(t, "NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	command := &cobra.Command{Use: "nore [command]", Short: "Manage Nore sites"}
	command.AddCommand(&cobra.Command{Use: "site", Short: "Manage sites"})
	if err := (Printer{Stdout: &stdout}).Help(command, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); !strings.Contains(output, "\x1b[") {
		t.Fatalf("expected ANSI color output: %q", output)
	}
}

func TestHelpHonorsNoColor(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	command := &cobra.Command{Use: "nore", Short: "Manage Nore sites"}
	if err := (Printer{NoColor: true, Stdout: &stdout}).Help(command, "dev"); err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); strings.Contains(output, "\x1b[") {
		t.Fatalf("expected plain output: %q", output)
	}
}

func TestStatusUsesColorWhenForced(t *testing.T) {
	restoreEnvironment(t, "NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	printer := Printer{Stdout: &stdout}
	if output := printer.Done("Ready"); !strings.Contains(output, "\x1b[") {
		t.Fatalf("expected ANSI color output: %q", output)
	}
}

func restoreEnvironment(t *testing.T, name string) {
	t.Helper()
	value, exists := os.LookupEnv(name)
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(name, value)
			return
		}
		_ = os.Unsetenv(name)
	})
}
