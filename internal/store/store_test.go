package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStoreRoundTripAndDelete(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nore", "credentials.json")
	store := Store{FilePath: path}
	want := Credentials{Registries: map[string]RegistryCredentials{
		"https://api.nore.sh": {OAuth: &OAuthCredentials{
			AccessExpiresAt:  "2026-07-17T12:00:00Z",
			AccessToken:      "access",
			RefreshExpiresAt: "2026-08-17T12:00:00Z",
			RefreshToken:     "refresh",
		}},
	}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	registryCredentials, err := got.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if registryCredentials.OAuth == nil || registryCredentials.OAuth.RefreshToken != "refresh" {
		t.Fatalf("Load() = %#v", got)
	}
	if err := store.Save(Credentials{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestStoreLoadPreservesExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use POSIX file modes")
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{
  "registries": {
    "https://api.nore.sh": {
      "manualToken": "token"
    }
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	store := Store{FilePath: path}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("credentials mode = %o", info.Mode().Perm())
	}
}

func TestStoreNormalizesRegistryNamespaces(t *testing.T) {
	t.Parallel()
	store := Store{FilePath: filepath.Join(t.TempDir(), "credentials.json")}
	if err := store.Save(Credentials{Registries: map[string]RegistryCredentials{
		"https://API.NORE.SH/": {ManualToken: " token "},
	}}); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials.Registries) != 1 || credentials.Registries["https://api.nore.sh"].ManualToken != "token" {
		t.Fatalf("Load() = %#v", credentials)
	}
}

func TestStoreRejectsDuplicateNormalizedRegistryNamespaces(t *testing.T) {
	t.Parallel()
	store := Store{FilePath: filepath.Join(t.TempDir(), "credentials.json")}
	err := store.Save(Credentials{Registries: map[string]RegistryCredentials{
		"https://api.nore.sh":  {ManualToken: "first"},
		"https://API.NORE.SH/": {ManualToken: "second"},
	}})
	if err == nil {
		t.Fatal("Save() accepted duplicate normalized registry namespaces")
	}
}

func TestMigrateCurrentFlatCredentials(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(`{
  "manualToken": "unscoped",
  "oauth": {
    "accessExpiresAt": "2026-07-17T12:00:00Z",
    "accessToken": "access",
    "refreshExpiresAt": "2026-08-17T12:00:00Z",
    "refreshToken": "refresh",
    "registry": "https://API.NORE.SH/"
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{FilePath: path}
	migrated, err := store.MigrateLegacy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Fatal("MigrateLegacy() = false")
	}
	credentials, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if credentials.LegacyManualToken != "unscoped" || credentials.LegacyOAuth != nil {
		t.Fatalf("Load() = %#v", credentials)
	}
	registryCredentials, err := credentials.ForRegistry("https://api.nore.sh")
	if err != nil {
		t.Fatal(err)
	}
	if registryCredentials.OAuth == nil || registryCredentials.OAuth.RefreshToken != "refresh" {
		t.Fatalf("Load() = %#v", credentials)
	}
}

func TestMigrateOAuthUsesDefaultRegistryWhenMissing(t *testing.T) {
	t.Parallel()
	registry, oauth, err := migrateOAuth(&legacyOAuthCredentials{
		AccessToken:      "access",
		RefreshExpiresAt: "2026-08-17T12:00:00Z",
		RefreshToken:     "refresh",
	})
	if err != nil {
		t.Fatal(err)
	}
	if registry != "https://api.nore.sh" || oauth.RefreshToken != "refresh" {
		t.Fatalf("migrateOAuth() = %q, %#v", registry, oauth)
	}
}

func TestCredentialLockHonorsContext(t *testing.T) {
	t.Parallel()
	store := Store{FilePath: filepath.Join(t.TempDir(), "credentials.json")}
	first, err := store.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := store.Lock(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock() error = %v", err)
	}
}

func TestParseLegacyOAuth(t *testing.T) {
	t.Parallel()
	credentials := parseLegacyOAuth(`{
  "accessExpiresAt": "2026-07-17T12:00:00Z",
  "accessToken": "access",
  "refreshExpiresAt": "2026-08-17T12:00:00Z",
  "refreshToken": "refresh",
  "registry": "https://api.nore.sh"
}`)
	if credentials == nil || credentials.RefreshToken != "refresh" {
		t.Fatalf("parseLegacyOAuth() = %#v", credentials)
	}
	if parseLegacyOAuth(`{"accessToken":"access"}`) != nil {
		t.Fatal("parseLegacyOAuth() accepted incomplete credentials")
	}
}
