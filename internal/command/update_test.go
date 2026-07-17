package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/norehq/cli/internal/buildinfo"
)

type fakeUpdateService struct {
	installCalls int
	installError error
	installation installationInfo
	release      releaseInfo
	releaseError error
}

func (f *fakeUpdateService) Install(_ context.Context, stdout io.Writer, _ io.Writer) error {
	f.installCalls++
	_, _ = fmt.Fprint(stdout, "installer output\n")
	return f.installError
}

func (f *fakeUpdateService) Installation() installationInfo {
	return f.installation
}

func (f *fakeUpdateService) LatestRelease(context.Context) (releaseInfo, error) {
	return f.release, f.releaseError
}

func TestUpdateAlreadyLatest(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		installation: installationInfo{Method: "standalone", Platform: "darwin"},
		release:      releaseInfo{Version: "1.2.3"},
	}
	stdout, stderr, err := executeUpdateCommand(service, "1.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	if service.installCalls != 0 {
		t.Fatalf("installCalls = %d", service.installCalls)
	}
	if !strings.Contains(stdout, "already the latest version") {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestUpdateNPMPrintsDetectedManagerCommand(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		installation: installationInfo{Manager: "pnpm", Method: "npm", Platform: "linux"},
		release:      releaseInfo{Version: "1.3.0"},
	}
	stdout, _, err := executeUpdateCommand(service, "1.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	if service.installCalls != 0 {
		t.Fatalf("installCalls = %d", service.installCalls)
	}
	if !strings.Contains(stdout, "pnpm add --global @norehq/cli@latest") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestUpdateNativeRunsForcedInstaller(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		installation: installationInfo{Method: "standalone", Platform: "linux"},
		release:      releaseInfo{Version: "1.3.0"},
	}
	stdout, _, err := executeUpdateCommand(service, "1.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	if service.installCalls != 1 {
		t.Fatalf("installCalls = %d", service.installCalls)
	}
	for _, expected := range []string{"Updating Nore CLI", "installer output", "v1.3.0 was installed"} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected stdout to contain %q: %q", expected, stdout)
		}
	}
}

func TestUpdateWindowsPrintsPowerShellInstaller(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		installation: installationInfo{Method: "standalone", Platform: "windows"},
		release:      releaseInfo{Version: "1.3.0"},
	}
	stdout, _, err := executeUpdateCommand(service, "1.2.3", false)
	if err != nil {
		t.Fatal(err)
	}
	if service.installCalls != 0 {
		t.Fatalf("installCalls = %d", service.installCalls)
	}
	if !strings.Contains(stdout, powershellInstall) {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestUpdateJSONCapturesInstallerOutput(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		installation: installationInfo{Method: "standalone", Platform: "darwin"},
		release:      releaseInfo{URL: "https://example.com/release", Version: "1.3.0"},
	}
	stdout, stderr, err := executeUpdateCommand(service, "1.2.3", true)
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	var result updateResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.LatestVersion != "1.3.0" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGitHubUpdateServiceLatestRelease(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", request.Header.Get("Accept"))
		}
		if request.Header.Get("User-Agent") != "nore-cli" {
			t.Errorf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		_ = json.NewEncoder(response).Encode(map[string]string{
			"html_url": "https://github.com/norehq/cli/releases/tag/v1.3.0",
			"tag_name": "v1.3.0",
		})
	}))
	defer server.Close()
	service := testGitHubUpdateService(server)
	release, err := service.LatestRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.3.0" || !strings.HasSuffix(release.URL, "/v1.3.0") {
		t.Fatalf("release = %#v", release)
	}
}

func TestGitHubUpdateServiceRunsInstallerWithForce(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, "test \"$1\" = \"--force\" || exit 12\nprintf 'forced installer'")
	}))
	defer server.Close()
	service := testGitHubUpdateService(server)
	service.installScriptURL = server.URL
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := service.Install(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("Install() error = %v, stderr = %q", err, stderr.String())
	}
	if stdout.String() != "forced installer" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestGitHubUpdateServiceDetectsNPMInstallation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		environment map[string]string
		executable  string
		manager     string
		method      string
		name        string
	}{
		{
			environment: map[string]string{"NORE_INSTALL_METHOD": "npm", "NORE_PACKAGE_MANAGER": "yarn"},
			manager:     "yarn",
			method:      "npm",
			name:        "wrapper metadata",
		},
		{
			executable: "/home/me/.local/share/pnpm/global/5/.pnpm/@norehq+cli-linux-x64/node_modules/@norehq/cli-linux-x64/bin/nore",
			manager:    "pnpm",
			method:     "npm",
			name:       "pnpm executable path",
		},
		{
			executable: "/home/me/.bun/install/global/node_modules/@norehq/cli-linux-x64/bin/nore",
			manager:    "bun",
			method:     "npm",
			name:       "bun executable path",
		},
		{
			executable: "/home/me/.local/bin/nore",
			method:     "standalone",
			name:       "standalone executable",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := &githubUpdateService{
				executable: func() (string, error) { return test.executable, nil },
				getenv:     func(name string) string { return test.environment[name] },
				platform:   "linux",
			}
			installation := service.Installation()
			if installation.Method != test.method || installation.Manager != test.manager {
				t.Fatalf("Installation() = %#v", installation)
			}
		})
	}
}

func TestNPMUpdateCommands(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"bun":  "bun add --global @norehq/cli@latest",
		"npm":  "npm install --global @norehq/cli@latest",
		"pnpm": "pnpm add --global @norehq/cli@latest",
		"yarn": "yarn global add @norehq/cli@latest",
	}
	for manager, expected := range tests {
		if command := npmUpdateCommand(manager); command != expected {
			t.Errorf("npmUpdateCommand(%q) = %q", manager, command)
		}
	}
}

func TestNewerVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		current string
		latest  string
		newer   bool
	}{
		{current: "1.2.3", latest: "1.2.4", newer: true},
		{current: "1.2.3", latest: "1.2.3"},
		{current: "1.3.0", latest: "1.2.4"},
		{current: "dev", latest: "1.2.4", newer: true},
		{current: "1.3.0-rc.1", latest: "1.3.0", newer: true},
	}
	for _, test := range tests {
		if result := newerVersion(test.current, test.latest); result != test.newer {
			t.Errorf("newerVersion(%q, %q) = %t", test.current, test.latest, result)
		}
	}
}

func executeUpdateCommand(
	service updateService,
	currentVersion string,
	jsonOutput bool,
) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	application := &app{
		buildInfo: buildinfo.Info{Version: currentVersion},
		json:      jsonOutput,
		noColor:   true,
		stderr:    &stderr,
		stdout:    &stdout,
		updater:   service,
	}
	command := application.updateCommand()
	command.SetArgs([]string{})
	err := command.Execute()
	return stdout.String(), stderr.String(), err
}

func testGitHubUpdateService(server *httptest.Server) *githubUpdateService {
	return &githubUpdateService{
		client:           server.Client(),
		executable:       func() (string, error) { return "/tmp/nore", nil },
		getenv:           func(string) string { return "" },
		installScriptURL: server.URL,
		latestReleaseURL: server.URL,
		platform:         "linux",
	}
}
