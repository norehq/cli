package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptForConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		confirmed bool
	}{
		{name: "lowercase y", input: "y\n", confirmed: true},
		{name: "uppercase yes", input: "YES\n", confirmed: true},
		{name: "lowercase n", input: "n\n", confirmed: false},
		{name: "uppercase no", input: "NO\n", confirmed: false},
		{name: "default", input: "\n", confirmed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			confirmed, err := promptForConfirmation(
				strings.NewReader(test.input),
				&output,
				"Continue?",
			)
			if err != nil {
				t.Fatal(err)
			}
			if confirmed != test.confirmed {
				t.Fatalf("confirmed = %t, want %t", confirmed, test.confirmed)
			}
			if output.String() != "Continue? [y/N] " {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func TestPromptForConfirmationRetriesInvalidInput(t *testing.T) {
	var output bytes.Buffer
	confirmed, err := promptForConfirmation(
		strings.NewReader("maybe\ny\n"),
		&output,
		"Continue?",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatal("confirmed = false, want true")
	}
	want := "Continue? [y/N] Please answer y or n.\nContinue? [y/N] "
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
