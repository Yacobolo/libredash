package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetEnvironmentDiscoversAndAssertsInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/instance" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request = %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"environment":"prod"}`))
	}))
	defer server.Close()
	if got, err := targetEnvironment(context.Background(), server.Client(), server.URL, "token", ""); err != nil || got != "prod" {
		t.Fatalf("environment = %q, %v", got, err)
	}
	if _, err := targetEnvironment(context.Background(), server.Client(), server.URL, "token", "staging"); err == nil {
		t.Fatal("mismatched assertion succeeded")
	}
}

func TestLoadClientConfigMakesExistingTokenFilePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.json")
	t.Setenv("LEAPVIEW_CLI_CONFIG", path)
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
