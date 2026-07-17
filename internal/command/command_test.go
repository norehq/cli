package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/norehq/cli/internal/config"
	"github.com/norehq/cli/internal/store"
)

func TestRootJSONHelp(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["command"] != "root" {
		t.Fatalf("command = %#v", result["command"])
	}
}

func TestRootHumanHelpUsesColorWhenForced(t *testing.T) {
	restoreTestEnvironment(t, "NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "\x1b[") {
		t.Fatalf("expected ANSI color output: %q", output)
	}
	for _, expected := range []string{"Usage", "Commands", "--no-color"} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("expected help output to contain %q: %q", expected, stdout.String())
		}
	}
}

func TestRootHumanHelpHonorsNoColor(t *testing.T) {
	restoreTestEnvironment(t, "NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"--help", "--no-color"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	if output := stdout.String(); strings.Contains(output, "\x1b[") {
		t.Fatalf("expected plain output: %q", output)
	}
}

func TestSiteListJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/sites" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer person-token" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"sites": []map[string]any{{"id": "site-id", "ident": "docs"}},
		})
	}))
	defer server.Close()
	configureTestEnvironment(t, server.URL)
	t.Setenv("NORE_TOKEN", "person-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(arrayValue(result["sites"])) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestLogoutRevokesAndDeletesOAuthCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/v1/cli/self/credential" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	_, credentialsPath := configureTestEnvironment(t, server.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{OAuth: &store.OAuthCredentials{
		AccessExpiresAt:  time.Now().Add(time.Hour).Format(time.RFC3339Nano),
		AccessToken:      "oauth-access",
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
		RefreshToken:     "oauth-refresh",
		Registry:         server.URL,
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"logout", "--yes", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	if _, err := credentials.Load(); err != store.ErrNotConfigured {
		t.Fatalf("credentials.Load() error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["remoteRevoked"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestReleaseOutcome(t *testing.T) {
	t.Parallel()
	if got := releaseOutcome(map[string]any{"status": "SKIPPED"}, false); got != "NO_CHANGES" {
		t.Fatalf("releaseOutcome() = %q", got)
	}
	if got := releaseOutcome(map[string]any{"status": "SKIPPED", "errorMessage": "failed"}, false); got != "FAILED" {
		t.Fatalf("releaseOutcome() = %q", got)
	}
	if got := releaseOutcome(map[string]any{"status": "RUNNING"}, true); got != "INDETERMINATE" {
		t.Fatalf("releaseOutcome() = %q", got)
	}
}

func configureTestEnvironment(t *testing.T, registry string) (string, string) {
	t.Helper()
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	credentialsPath := filepath.Join(directory, "credentials.json")
	if _, err := (config.Store{FilePath: configPath}).SetRegistry(registry); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NORE_CONFIG_PATH", configPath)
	t.Setenv("NORE_CREDENTIALS_PATH", credentialsPath)
	t.Setenv("NORE_HOME", "")
	t.Setenv("NORE_TOKEN", "")
	return configPath, credentialsPath
}

func restoreTestEnvironment(t *testing.T, name string) {
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
