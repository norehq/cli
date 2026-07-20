package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/norehq/cli/internal/config"
)

const (
	legacyKeychainAccount = "oauth"
	legacyKeychainService = "im.nore.cli"
	legacyCommandTimeout  = 3 * time.Second
)

type legacyConfig struct {
	Token string `json:"token"`
}

type legacyOAuthCredentials struct {
	AccessExpiresAt  string `json:"accessExpiresAt"`
	AccessToken      string `json:"accessToken"`
	RefreshExpiresAt string `json:"refreshExpiresAt"`
	RefreshToken     string `json:"refreshToken"`
	Registry         string `json:"registry"`
}

func (s Store) MigrateLegacy(ctx context.Context) (bool, error) {
	credentials, err := s.Load()
	if err == nil {
		if credentials.LegacyOAuth == nil {
			return false, nil
		}
		registry, oauth, err := migrateOAuth(credentials.LegacyOAuth)
		if err != nil {
			return false, err
		}
		registryCredentials, err := credentials.ForRegistry(registry)
		if err != nil {
			return false, err
		}
		if registryCredentials.OAuth != nil {
			return false, errors.New("legacy OAuth credentials conflict with registry credentials")
		}
		registryCredentials.OAuth = oauth
		if err := credentials.SetRegistry(registry, registryCredentials); err != nil {
			return false, err
		}
		credentials.LegacyOAuth = nil
		if err := s.Save(credentials); err != nil {
			return false, err
		}
		return true, nil
	}
	if !errors.Is(err, ErrNotConfigured) {
		return false, err
	}
	if !s.legacyMigrationAllowed() {
		return false, nil
	}
	directory, err := legacyDirectory()
	if err != nil {
		return false, err
	}
	configPath := filepath.Join(directory, "config.json")
	credentialsPath := filepath.Join(directory, "credentials.json")
	manualToken := legacyManualToken(configPath)
	oauth, _ := legacyOAuth(ctx, credentialsPath)
	credentials = Credentials{LegacyManualToken: manualToken}
	if oauth != nil {
		registry, migratedOAuth, err := migrateOAuth(oauth)
		if err != nil {
			return false, err
		}
		if err := credentials.SetRegistry(registry, RegistryCredentials{
			OAuth: migratedOAuth,
		}); err != nil {
			return false, err
		}
	}
	if credentials.Empty() {
		return false, nil
	}
	if err := s.Save(credentials); err != nil {
		return false, err
	}
	_ = removeLegacyFile(s, configPath)
	_ = removeLegacyFile(s, credentialsPath)
	deleteLegacyKeychain(ctx)
	return true, nil
}

func migrateOAuth(credentials *legacyOAuthCredentials) (string, *OAuthCredentials, error) {
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.RefreshExpiresAt == "" {
		return "", nil, errors.New("legacy OAuth credentials are incomplete")
	}
	registryValue := credentials.Registry
	if strings.TrimSpace(registryValue) == "" {
		registryValue = config.DefaultRegistry
	}
	registry, err := config.NormalizeRegistry(registryValue)
	if err != nil {
		return "", nil, err
	}
	return registry, &OAuthCredentials{
		AccessExpiresAt:  credentials.AccessExpiresAt,
		AccessToken:      credentials.AccessToken,
		RefreshExpiresAt: credentials.RefreshExpiresAt,
		RefreshToken:     credentials.RefreshToken,
	}, nil
}

func (s Store) legacyMigrationAllowed() bool {
	return strings.TrimSpace(s.FilePath) == "" &&
		strings.TrimSpace(os.Getenv(credentialsPathEnvironment)) == "" &&
		strings.TrimSpace(os.Getenv("NORE_HOME")) == "" &&
		strings.TrimSpace(os.Getenv("NORE_CONFIG_PATH")) == ""
}

func legacyDirectory() (string, error) {
	if directory := strings.TrimSpace(os.Getenv("NORE_CONFIG_DIR")); directory != "" {
		return filepath.Clean(directory), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nore"), nil
	case "windows":
		appData := strings.TrimSpace(os.Getenv("APPDATA"))
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "nore"), nil
	default:
		configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "nore"), nil
	}
}

func legacyManualToken(path string) string {
	payload, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var config legacyConfig
	if json.Unmarshal(payload, &config) != nil {
		return ""
	}
	return strings.TrimSpace(config.Token)
}

func legacyOAuth(ctx context.Context, path string) (*legacyOAuthCredentials, string) {
	if serialized := readLegacyKeychain(ctx); serialized != "" {
		if credentials := parseLegacyOAuth(serialized); credentials != nil {
			return credentials, "keychain"
		}
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	return parseLegacyOAuth(string(payload)), "file"
}

func parseLegacyOAuth(value string) *legacyOAuthCredentials {
	var credentials legacyOAuthCredentials
	if json.Unmarshal([]byte(strings.TrimSpace(value)), &credentials) != nil {
		return nil
	}
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.RefreshExpiresAt == "" {
		return nil
	}
	return &credentials
}

func readLegacyKeychain(parent context.Context) string {
	ctx, cancel := context.WithTimeout(parent, legacyCommandTimeout)
	defer cancel()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(
			ctx,
			"security",
			"find-generic-password",
			"-a",
			legacyKeychainAccount,
			"-s",
			legacyKeychainService,
			"-w",
		)
	case "linux":
		command = exec.CommandContext(
			ctx,
			"secret-tool",
			"lookup",
			"service",
			legacyKeychainService,
			"account",
			legacyKeychainAccount,
		)
	default:
		return ""
	}
	payload, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(payload))
}

func deleteLegacyKeychain(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, legacyCommandTimeout)
	defer cancel()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(
			ctx,
			"security",
			"delete-generic-password",
			"-a",
			legacyKeychainAccount,
			"-s",
			legacyKeychainService,
		)
	case "linux":
		command = exec.CommandContext(
			ctx,
			"secret-tool",
			"clear",
			"service",
			legacyKeychainService,
			"account",
			legacyKeychainAccount,
		)
	default:
		return
	}
	_ = command.Run()
}

func removeLegacyFile(store Store, legacyPath string) error {
	currentPath, err := store.Path()
	if err != nil {
		return err
	}
	if filepath.Clean(currentPath) == filepath.Clean(legacyPath) {
		return nil
	}
	err = os.Remove(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
