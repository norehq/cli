package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const (
	githubLatestReleaseURL = "https://api.github.com/repos/norehq/cli/releases/latest"
	installScriptURL       = "https://nore.sh/install.sh"
	powershellInstall      = "irm https://nore.sh/install.ps1 | iex"
	updateHTTPTimeout      = 30 * time.Second
	updateTimeout          = 5 * time.Minute
)

type releaseInfo struct {
	URL     string
	Version string
}

type installationInfo struct {
	Manager  string
	Method   string
	Platform string
}

type updateResult struct {
	Command         string `json:"command,omitempty"`
	CurrentVersion  string `json:"currentVersion"`
	InstallMethod   string `json:"installMethod"`
	LatestVersion   string `json:"latestVersion"`
	PackageManager  string `json:"packageManager,omitempty"`
	ReleaseURL      string `json:"releaseUrl,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Updated         bool   `json:"updated"`
}

type updateService interface {
	Install(context.Context, io.Writer, io.Writer) error
	Installation() installationInfo
	LatestRelease(context.Context) (releaseInfo, error)
}

type githubUpdateService struct {
	client           *http.Client
	executable       func() (string, error)
	getenv           func(string) string
	installScriptURL string
	latestReleaseURL string
	platform         string
}

func newGitHubUpdateService() updateService {
	return &githubUpdateService{
		client:           &http.Client{Timeout: updateHTTPTimeout},
		executable:       os.Executable,
		getenv:           os.Getenv,
		installScriptURL: installScriptURL,
		latestReleaseURL: githubLatestReleaseURL,
		platform:         runtime.GOOS,
	}
}

func (a *app) updateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update Nore CLI to the latest release",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(command.Context(), updateTimeout)
			defer cancel()
			release, err := a.updater.LatestRelease(ctx)
			if err != nil {
				return newCommandError(
					"UPDATE_CHECK_FAILED",
					"The latest Nore CLI release could not be checked.",
					err.Error(),
				)
			}
			installation := a.updater.Installation()
			result := updateResult{
				CurrentVersion: a.buildInfo.Version,
				InstallMethod:  installation.Method,
				LatestVersion:  release.Version,
				PackageManager: installation.Manager,
				ReleaseURL:     release.URL,
			}
			if !newerVersion(a.buildInfo.Version, release.Version) {
				return a.printer().Success(
					result,
					fmt.Sprintf(
						"Nore CLI %s is already the latest version.",
						displayVersion(a.buildInfo.Version),
					),
				)
			}
			result.UpdateAvailable = true
			if installation.Method == "npm" {
				result.Command = npmUpdateCommand(installation.Manager)
				human := fmt.Sprintf(
					"Nore CLI %s is available (current: %s).\nThis installation is managed by %s. Update it with:\n\n  %s",
					displayVersion(release.Version),
					displayVersion(a.buildInfo.Version),
					installation.Manager,
					result.Command,
				)
				return a.printer().Success(result, human)
			}
			if installation.Platform == "windows" {
				result.Command = powershellInstall
				human := fmt.Sprintf(
					"Nore CLI %s is available (current: %s). Run this in PowerShell to update:\n\n  %s",
					displayVersion(release.Version),
					displayVersion(a.buildInfo.Version),
					result.Command,
				)
				return a.printer().Success(result, human)
			}
			if installation.Platform != "darwin" && installation.Platform != "linux" {
				return newCommandError(
					"UPDATE_UNSUPPORTED",
					"This platform cannot be updated automatically.",
					map[string]string{"platform": installation.Platform, "releaseUrl": release.URL},
				)
			}
			if !a.json {
				a.printer().Progress(
					map[string]any{"currentVersion": a.buildInfo.Version, "latestVersion": release.Version},
					fmt.Sprintf("Updating Nore CLI to %s...", displayVersion(release.Version)),
				)
			}
			var capturedStdout bytes.Buffer
			var capturedStderr bytes.Buffer
			installerStdout := a.stdout
			installerStderr := a.stderr
			if a.json {
				installerStdout = &capturedStdout
				installerStderr = &capturedStderr
			}
			if err := a.updater.Install(ctx, installerStdout, installerStderr); err != nil {
				return newCommandError(
					"UPDATE_FAILED",
					"Nore CLI could not be updated.",
					map[string]string{
						"error":  err.Error(),
						"stderr": strings.TrimSpace(capturedStderr.String()),
						"stdout": strings.TrimSpace(capturedStdout.String()),
					},
				)
			}
			result.Updated = true
			return a.printer().Success(
				result,
				fmt.Sprintf("Nore CLI %s was installed.", displayVersion(release.Version)),
			)
		},
	}
}

func (u *githubUpdateService) LatestRelease(ctx context.Context) (releaseInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.latestReleaseURL, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "nore-cli")
	response, err := u.client.Do(request)
	if err != nil {
		return releaseInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return releaseInfo{}, fmt.Errorf("GitHub returned %s", response.Status)
	}
	var payload struct {
		HTMLURL string `json:"html_url"`
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return releaseInfo{}, err
	}
	version := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if !semver.IsValid("v" + version) {
		return releaseInfo{}, fmt.Errorf("GitHub returned an invalid release version %q", payload.TagName)
	}
	return releaseInfo{URL: strings.TrimSpace(payload.HTMLURL), Version: version}, nil
}

func (u *githubUpdateService) Installation() installationInfo {
	executable, _ := u.executable()
	method := strings.ToLower(strings.TrimSpace(u.getenv("NORE_INSTALL_METHOD")))
	if method == "npm" || isNPMExecutable(executable) {
		return installationInfo{
			Manager:  detectedPackageManager(u.getenv, executable),
			Method:   "npm",
			Platform: u.platform,
		}
	}
	return installationInfo{Method: "standalone", Platform: u.platform}
}

func (u *githubUpdateService) Install(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.installScriptURL, nil)
	if err != nil {
		return err
	}
	response, err := u.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("installer returned %s", response.Status)
	}
	process := exec.CommandContext(ctx, "sh", "-s", "--", "--force")
	process.Stdin = response.Body
	process.Stdout = stdout
	process.Stderr = stderr
	if err := process.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	return nil
}

func newerVersion(current string, latest string) bool {
	latestSemver := semanticVersion(latest)
	if !semver.IsValid(latestSemver) {
		return false
	}
	currentSemver := semanticVersion(current)
	if !semver.IsValid(currentSemver) {
		return true
	}
	return semver.Compare(latestSemver, currentSemver) > 0
}

func semanticVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	return "v" + value
}

func displayVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "dev" {
		return "dev"
	}
	if strings.HasPrefix(value, "v") {
		return value
	}
	return "v" + value
}

func isNPMExecutable(executable string) bool {
	path := strings.ToLower(strings.ReplaceAll(executable, "\\", "/"))
	return strings.Contains(path, "/node_modules/@norehq/cli-")
}

func detectedPackageManager(getenv func(string) string, executable string) string {
	if manager := packageManagerName(getenv("NORE_PACKAGE_MANAGER")); manager != "" {
		return manager
	}
	if manager := packageManagerName(getenv("npm_config_user_agent")); manager != "" {
		return manager
	}
	paths := strings.ToLower(strings.ReplaceAll(strings.Join([]string{
		executable,
		getenv("npm_execpath"),
		getenv("PNPM_HOME"),
		getenv("BUN_INSTALL"),
	}, "\n"), "\\", "/"))
	switch {
	case strings.Contains(paths, "pnpm"):
		return "pnpm"
	case strings.Contains(paths, "/.bun/") || strings.Contains(paths, "/bun/install/"):
		return "bun"
	case strings.Contains(paths, "/.yarn/") || strings.Contains(paths, "/yarn/"):
		return "yarn"
	default:
		return "npm"
	}
}

func packageManagerName(value string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if index := strings.IndexAny(name, "/@"); index >= 0 {
		name = name[:index]
	}
	switch name {
	case "bun", "npm", "pnpm", "yarn":
		return name
	default:
		return ""
	}
}

func npmUpdateCommand(manager string) string {
	switch manager {
	case "bun":
		return "bun add --global @norehq/cli@latest"
	case "pnpm":
		return "pnpm add --global @norehq/cli@latest"
	case "yarn":
		return "yarn global add @norehq/cli@latest"
	default:
		return "npm install --global @norehq/cli@latest"
	}
}
