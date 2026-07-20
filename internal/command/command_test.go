package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		server.URL:            oauthRegistryCredentials("oauth-access", "oauth-refresh"),
		"https://api.nore.sh": oauthRegistryCredentials("production-access", "production-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"logout", "--yes", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	saved, err := credentials.Load()
	if err != nil {
		t.Fatal(err)
	}
	current, err := saved.ForRegistry(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	production, err := saved.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if current.OAuth != nil || production.OAuth == nil || production.OAuth.AccessToken != "production-access" {
		t.Fatalf("credentials.Load() = %#v", saved)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["remoteRevoked"] != true {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(stdout.String(), server.URL) || strings.Contains(stderr.String(), server.URL) {
		t.Fatalf("command output exposed registry: stdout = %s, stderr = %s", stdout.String(), stderr.String())
	}
}

func TestAuthenticatedCommandsDoNotUseOrPrintAnotherRegistry(t *testing.T) {
	var configuredRequests atomic.Int32
	configuredServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		configuredRequests.Add(1)
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer configuredServer.Close()
	var otherRequests atomic.Int32
	otherServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		otherRequests.Add(1)
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer otherServer.Close()
	_, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		otherServer.URL: oauthRegistryCredentials("other-access", "other-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"site", "list", "--json"},
		{"whoami", "--json"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exitCode := Execute(args, &stdout, &stderr); exitCode == 0 {
			t.Fatalf("Execute(%q) = %d, stdout = %s", args, exitCode, stdout.String())
		}
		if strings.Contains(stderr.String(), configuredServer.URL) || strings.Contains(stderr.String(), otherServer.URL) {
			t.Fatalf("Execute(%q) exposed registry: %s", args, stderr.String())
		}
		if !strings.Contains(stderr.String(), "current registry") {
			t.Fatalf("Execute(%q) stderr = %s", args, stderr.String())
		}
	}
	if configuredRequests.Load() != 0 || otherRequests.Load() != 0 {
		t.Fatalf("requests = configured:%d other:%d", configuredRequests.Load(), otherRequests.Load())
	}
}

func TestWhoamiNetworkErrorDoesNotPrintRegistryEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	registry := server.URL
	server.Close()
	configureTestEnvironment(t, registry)
	t.Setenv("NORE_TOKEN", "person-token")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"whoami", "--json"}, &stdout, &stderr); exitCode == 0 {
		t.Fatalf("Execute() = %d, stdout = %s", exitCode, stdout.String())
	}
	if strings.Contains(stderr.String(), registry) {
		t.Fatalf("command output exposed registry: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "NETWORK_ERROR") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestSiteListSelectsConfiguredRegistryCredentials(t *testing.T) {
	var configuredRequests atomic.Int32
	configuredServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		configuredRequests.Add(1)
		if request.Header.Get("Authorization") != "Bearer configured-access" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
	}))
	defer configuredServer.Close()
	var otherRequests atomic.Int32
	otherServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		otherRequests.Add(1)
		response.WriteHeader(http.StatusInternalServerError)
	}))
	defer otherServer.Close()
	_, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		configuredServer.URL: oauthRegistryCredentials("configured-access", "configured-refresh"),
		otherServer.URL:      oauthRegistryCredentials("other-access", "other-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	if configuredRequests.Load() != 1 || otherRequests.Load() != 0 {
		t.Fatalf("requests = configured:%d other:%d", configuredRequests.Load(), otherRequests.Load())
	}
}

func TestValidOAuthTokenDoesNotAcquireCredentialLock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
	}))
	defer server.Close()
	_, credentialsPath := configureTestEnvironment(t, server.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		server.URL: oauthRegistryCredentials("oauth-access", "oauth-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(credentialsPath+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
}

func TestOAuthRefreshUpdatesOnlyConfiguredRegistry(t *testing.T) {
	configuredServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli-auth/token":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"accessToken":      "refreshed-access",
				"expiresIn":        3600,
				"refreshExpiresAt": time.Now().Add(48 * time.Hour).Format(time.RFC3339Nano),
				"refreshToken":     "refreshed-refresh",
				"tokenType":        "Bearer",
			})
		case "/v1/cli/sites":
			if request.Header.Get("Authorization") != "Bearer refreshed-access" {
				t.Errorf("authorization = %q", request.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
		default:
			t.Errorf("path = %q", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer configuredServer.Close()
	_, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	credentials := store.Store{FilePath: credentialsPath}
	configured := oauthRegistryCredentials("expired-access", "configured-refresh")
	configured.OAuth.AccessExpiresAt = time.Now().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		configuredServer.URL:  configured,
		"https://api.nore.sh": oauthRegistryCredentials("production-access", "production-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	saved, err := credentials.Load()
	if err != nil {
		t.Fatal(err)
	}
	current, err := saved.ForRegistry(configuredServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	production, err := saved.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if current.OAuth == nil || current.OAuth.RefreshToken != "refreshed-refresh" {
		t.Fatalf("current credentials = %#v", current)
	}
	if production.OAuth == nil || production.OAuth.RefreshToken != "production-refresh" {
		t.Fatalf("production credentials = %#v", production)
	}
}

func TestConfigSetTokenScopesTokenToConfiguredRegistry(t *testing.T) {
	configuredServer := httptest.NewServer(http.NotFoundHandler())
	defer configuredServer.Close()
	_, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{
		Registries: map[string]store.RegistryCredentials{
			configuredServer.URL:  oauthRegistryCredentials("configured-access", "configured-refresh"),
			"https://api.nore.sh": oauthRegistryCredentials("production-access", "production-refresh"),
		},
		LegacyManualToken: "unscoped",
	}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"config", "set", "--token", "configured-token", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	saved, err := credentials.Load()
	if err != nil {
		t.Fatal(err)
	}
	current, err := saved.ForRegistry(configuredServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	production, err := saved.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if current.ManualToken != "configured-token" || current.OAuth != nil || saved.LegacyManualToken != "" {
		t.Fatalf("current credentials = %#v, all = %#v", current, saved)
	}
	if production.OAuth == nil || production.OAuth.AccessToken != "production-access" {
		t.Fatalf("production credentials = %#v", production)
	}
}

func TestConfigSetRegistryAndTokenFlagsScopeTokenToNewRegistry(t *testing.T) {
	configuredServer := httptest.NewServer(http.NotFoundHandler())
	defer configuredServer.Close()
	newServer := httptest.NewServer(http.NotFoundHandler())
	defer newServer.Close()
	configPath, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{
		"config", "set",
		"--registry", newServer.URL,
		"--token", "new-token",
		"--json",
	}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["registry"] != newServer.URL || result["configured"] != true || result["source"] != "credentials" {
		t.Fatalf("result = %#v", result)
	}
	values, err := (config.Store{FilePath: configPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if values.EffectiveRegistry() != newServer.URL {
		t.Fatalf("registry = %q", values.EffectiveRegistry())
	}
	credentials, err := (store.Store{FilePath: credentialsPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	configured, err := credentials.ForRegistry(configuredServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := credentials.ForRegistry(newServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if configured.ManualToken != "" || updated.ManualToken != "new-token" {
		t.Fatalf("credentials = %#v", credentials)
	}
}

func TestConfigGetAndResetRegistryFlags(t *testing.T) {
	configuredServer := httptest.NewServer(http.NotFoundHandler())
	defer configuredServer.Close()
	configPath, _ := configureTestEnvironment(t, configuredServer.URL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"config", "get", "--registry", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute(get) = %d, stderr = %s", exitCode, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["registry"] != configuredServer.URL {
		t.Fatalf("result = %#v", result)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := Execute([]string{"config", "reset", "--registry", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute(reset) = %d, stderr = %s", exitCode, stderr.String())
	}
	values, err := (config.Store{FilePath: configPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if values.EffectiveRegistry() != config.DefaultRegistry {
		t.Fatalf("registry = %q", values.EffectiveRegistry())
	}
}

func TestConfigValueCommandsRejectMissingFlagsAndLegacyPositionalSyntax(t *testing.T) {
	configureTestEnvironment(t, config.DefaultRegistry)
	for _, args := range [][]string{
		{"config", "get"},
		{"config", "set"},
		{"config", "unset"},
		{"config", "reset"},
		{"config", "get", "registry"},
		{"config", "set", "registry", "https://api.nore.sh"},
		{"config", "set", "token", "configured-token"},
		{"config", "unset", "token"},
		{"config", "reset", "registry"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exitCode := Execute(args, &stdout, &stderr); exitCode == 0 {
			t.Fatalf("Execute(%q) = %d, stdout = %s", args, exitCode, stdout.String())
		}
	}
}

func TestConfigUnsetTokenOnlyChangesConfiguredRegistry(t *testing.T) {
	configuredServer := httptest.NewServer(http.NotFoundHandler())
	defer configuredServer.Close()
	_, credentialsPath := configureTestEnvironment(t, configuredServer.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		configuredServer.URL:  {ManualToken: "configured-token"},
		"https://api.nore.sh": {ManualToken: "production-token"},
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"config", "unset", "--token", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	saved, err := credentials.Load()
	if err != nil {
		t.Fatal(err)
	}
	current, err := saved.ForRegistry(configuredServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	production, err := saved.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if current.ManualToken != "" || production.ManualToken != "production-token" {
		t.Fatalf("credentials = %#v", saved)
	}
}

func TestUnscopedManualTokenRequiresExplicitConfiguration(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	_, credentialsPath := configureTestEnvironment(t, server.URL)
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{LegacyManualToken: "unscoped"}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode == 0 {
		t.Fatalf("Execute() = %d, stdout = %s", exitCode, stdout.String())
	}
	if requests.Load() != 0 || !strings.Contains(stderr.String(), "TOKEN_REGISTRY_REQUIRED") {
		t.Fatalf("requests = %d, stderr = %s", requests.Load(), stderr.String())
	}
	if strings.Contains(stderr.String(), server.URL) {
		t.Fatalf("command output exposed registry: %s", stderr.String())
	}
}

func TestAuthenticatedCommandDoesNotMigrateLegacyCredentials(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(response).Encode(map[string]any{"sites": []any{}})
	}))
	defer server.Close()
	_, credentialsPath := configureTestEnvironment(t, server.URL)
	payload, err := json.Marshal(map[string]any{
		"oauth": map[string]any{
			"accessExpiresAt":  time.Now().Add(time.Hour).Format(time.RFC3339Nano),
			"accessToken":      "legacy-access",
			"refreshExpiresAt": time.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
			"refreshToken":     "legacy-refresh",
			"registry":         server.URL,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialsPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"site", "list", "--json"}, &stdout, &stderr); exitCode == 0 {
		t.Fatalf("Execute() = %d, stdout = %s", exitCode, stdout.String())
	}
	if requests.Load() != 0 || !strings.Contains(stderr.String(), "NOT_LOGGED_IN") {
		t.Fatalf("requests = %d, stderr = %s", requests.Load(), stderr.String())
	}
	after, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, payload) {
		t.Fatalf("credentials were migrated: %s", after)
	}
}

func TestConfigShowIgnoresCredentialRegistry(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	credentialsPath := filepath.Join(directory, "credentials.json")
	t.Setenv("NORE_CONFIG_PATH", configPath)
	t.Setenv("NORE_CREDENTIALS_PATH", credentialsPath)
	t.Setenv("NORE_HOME", "")
	t.Setenv("NORE_TOKEN", "")
	credentials := store.Store{FilePath: credentialsPath}
	if err := credentials.Save(store.Credentials{Registries: map[string]store.RegistryCredentials{
		"http://127.0.0.1:3001": oauthRegistryCredentials("local-access", "local-refresh"),
	}}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exitCode := Execute([]string{"config", "show", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("Execute() = %d, stderr = %s", exitCode, stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["registry"] != config.DefaultRegistry || result["source"] != "default" {
		t.Fatalf("result = %#v", result)
	}
}

func TestCommandsDoNotAcceptRegistryFlag(t *testing.T) {
	configureTestEnvironment(t, config.DefaultRegistry)
	for _, args := range [][]string{
		{"login", "--registry", "http://127.0.0.1:3001"},
		{"site", "list", "--registry", "http://127.0.0.1:3001"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exitCode := Execute(args, &stdout, &stderr); exitCode == 0 {
			t.Fatalf("Execute(%q) = %d, stdout = %s", args, exitCode, stdout.String())
		}
		if !strings.Contains(stderr.String(), "unknown flag: --registry") {
			t.Fatalf("Execute(%q) stderr = %s", args, stderr.String())
		}
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

func oauthRegistryCredentials(accessToken string, refreshToken string) store.RegistryCredentials {
	return store.RegistryCredentials{OAuth: &store.OAuthCredentials{
		AccessExpiresAt:  time.Now().Add(time.Hour).Format(time.RFC3339Nano),
		AccessToken:      accessToken,
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339Nano),
		RefreshToken:     refreshToken,
	}}
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
