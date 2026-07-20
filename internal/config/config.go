package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/norehq/cli/internal/home"
)

const (
	DefaultRegistry = "https://api.nore.sh"
	pathEnvironment = "NORE_CONFIG_PATH"
	directoryMode   = 0o700
)

var ErrInvalidRegistry = errors.New("invalid registry")

type Values struct {
	Registry string `json:"registry,omitempty"`
}

type Store struct {
	FilePath string
}

func DefaultStore() Store {
	return Store{}
}

func (s Store) Load() (Values, error) {
	path, err := s.Path()
	if err != nil {
		return Values{}, err
	}
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Values{}, nil
	}
	if err != nil {
		return Values{}, err
	}
	var values Values
	if err := json.Unmarshal(payload, &values); err != nil {
		return Values{}, fmt.Errorf("parse config: %w", err)
	}
	if values.Registry == "" {
		return values, nil
	}
	values.Registry, err = NormalizeRegistry(values.Registry)
	if err != nil {
		return Values{}, err
	}
	return values, nil
}

func (s Store) SetRegistry(value string) (string, error) {
	registry, err := NormalizeRegistry(value)
	if err != nil {
		return "", err
	}
	if err := s.write(Values{Registry: registry}); err != nil {
		return "", err
	}
	return registry, nil
}

func (s Store) ResetRegistry() error {
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
	if path := strings.TrimSpace(os.Getenv(pathEnvironment)); path != "" {
		return path, nil
	}
	return home.Path("config.json")
}

func (v Values) EffectiveRegistry() string {
	if v.Registry == "" {
		return DefaultRegistry
	}
	return v.Registry
}

func NormalizeRegistry(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidRegistry, err)
	}
	hostname := strings.ToLower(parsed.Hostname())
	loopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	validScheme := parsed.Scheme == "https" || (parsed.Scheme == "http" && loopback)
	if value == "" || !validScheme || parsed.Hostname() == "" || parsed.User != nil {
		return "", ErrInvalidRegistry
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (s Store) write(values Values) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
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
