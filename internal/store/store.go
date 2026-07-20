package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/norehq/cli/internal/home"
)

const (
	credentialsPathEnvironment = "NORE_CREDENTIALS_PATH"
	directoryMode              = 0o700
)

var ErrNotConfigured = errors.New("no saved credentials")

type OAuthCredentials struct {
	AccessExpiresAt  string `json:"accessExpiresAt"`
	AccessToken      string `json:"accessToken"`
	RefreshExpiresAt string `json:"refreshExpiresAt"`
	RefreshToken     string `json:"refreshToken"`
	Registry         string `json:"registry"`
}

type Credentials struct {
	ManualToken string            `json:"manualToken,omitempty"`
	OAuth       *OAuthCredentials `json:"oauth,omitempty"`
}

type Store struct {
	FilePath string
}

func DefaultStore() Store {
	return Store{}
}

func (s Store) Load() (Credentials, error) {
	path, err := s.Path()
	if err != nil {
		return Credentials{}, err
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrNotConfigured
	}
	if err != nil {
		return Credentials{}, err
	}
	var credentials Credentials
	if err := json.Unmarshal(payload, &credentials); err != nil {
		return Credentials{}, err
	}
	if credentials.ManualToken == "" && credentials.OAuth == nil {
		return Credentials{}, ErrNotConfigured
	}
	return credentials, nil
}

func (s Store) Save(credentials Credentials) error {
	if strings.TrimSpace(credentials.ManualToken) == "" && credentials.OAuth == nil {
		return s.Delete()
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(directory, ".credentials-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if _, err := temporary.Write(payload); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func (s Store) Delete() error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s Store) Path() (string, error) {
	if strings.TrimSpace(s.FilePath) != "" {
		return s.FilePath, nil
	}
	if path := strings.TrimSpace(os.Getenv(credentialsPathEnvironment)); path != "" {
		return path, nil
	}
	return home.Path("credentials.json")
}
