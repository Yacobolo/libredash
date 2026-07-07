package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadClientConfigMakesExistingTokenFilePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.json")
	t.Setenv("LIBREDASH_CLI_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"targets":{"http://localhost:8080":{"token":"secret"}}}`), 0o644); err != nil {
		t.Fatalf("write client config: %v", err)
	}

	config, err := loadClientConfig()
	if err != nil {
		t.Fatalf("load client config: %v", err)
	}
	if got := config.Targets["http://localhost:8080"].Token; got != "secret" {
		t.Fatalf("token = %q, want secret", got)
	}
	assertMode(t, path, 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %#o, want %#o", path, got, want)
	}
}
