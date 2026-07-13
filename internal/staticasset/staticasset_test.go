package staticasset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionDefaultsToDevOutsideProduction(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "")
	t.Setenv("LIBREDASH_ASSET_VERSION", "")

	if got := Version(); got != "dev" {
		t.Fatalf("Version = %q, want dev", got)
	}
}

func TestVersionUsesConfiguredEnvOverride(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	t.Setenv("LIBREDASH_ASSET_VERSION", "release-123")

	if got := URL("/static/app-shell.js"); got != "/static/app-shell.js?v=release-123" {
		t.Fatalf("URL = %q, want configured version", got)
	}
}

func TestVersionUsesGeneratedFileInProduction(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, generatedVersionPath), []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	t.Setenv("LIBREDASH_ASSET_VERSION", "")

	if got := URL("/static/app-shell.js"); got != "/static/app-shell.js?v=abc123" {
		t.Fatalf("URL = %q, want generated version", got)
	}
}

func TestProductionParsesBooleanEnv(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "true")
	if !Production() {
		t.Fatal("Production() = false, want true")
	}

	t.Setenv("LIBREDASH_PRODUCTION", "not-bool")
	if Production() {
		t.Fatal("Production() = true for invalid boolean, want false")
	}
}
