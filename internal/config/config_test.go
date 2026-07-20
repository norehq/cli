package config

import (
	"path/filepath"
	"testing"
)

func TestNormalizeRegistry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
		valid bool
	}{
		{input: "https://API.NORE.SH/", want: "https://api.nore.sh", valid: true},
		{input: "http://127.0.0.1:3001/", want: "http://127.0.0.1:3001", valid: true},
		{input: "http://LOCALHOST:3001/", want: "http://localhost:3001", valid: true},
		{input: "http://localhost:3001/path/?debug=1", want: "http://localhost:3001/path", valid: true},
		{input: "http://api.nore.sh", valid: false},
		{input: "https://user@example.com", valid: false},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := NormalizeRegistry(test.input)
			if test.valid && err != nil {
				t.Fatalf("NormalizeRegistry() error = %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("NormalizeRegistry() expected an error")
			}
			if got != test.want {
				t.Fatalf("NormalizeRegistry() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nore", "config.json")
	store := Store{FilePath: path}
	registry, err := store.SetRegistry("https://api.nore.sh/")
	if err != nil {
		t.Fatal(err)
	}
	if registry != DefaultRegistry {
		t.Fatalf("SetRegistry() = %q", registry)
	}
	values, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if values.Registry != DefaultRegistry {
		t.Fatalf("Load().Registry = %q", values.Registry)
	}
}
