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
	want := Credentials{OAuth: &OAuthCredentials{
		AccessExpiresAt:  "2026-07-17T12:00:00Z",
		AccessToken:      "access",
		RefreshExpiresAt: "2026-08-17T12:00:00Z",
		RefreshToken:     "refresh",
		Registry:         "https://api.nore.sh",
	}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.OAuth == nil || got.OAuth.RefreshToken != want.OAuth.RefreshToken {
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
	if err := os.WriteFile(path, []byte(`{"manualToken":"token"}`), 0o644); err != nil {
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
