package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckCommandRequiresReadyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			t.Fatalf("path = %q, want /readyz", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer server.Close()

	opts := &rootOptions{healthcheckURL: server.URL + "/readyz"}
	cmd := healthcheckCommand(context.Background(), opts)
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("healthcheck failed: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "ready" {
		t.Fatalf("output = %q, want ready", got)
	}
}

func TestHealthcheckCommandFailsOnUnreadyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	opts := &rootOptions{healthcheckURL: server.URL + "/readyz"}
	cmd := healthcheckCommand(context.Background(), opts)

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("error = %v, want status 503", err)
	}
}

func TestHealthcheckURLDefaultsFromListenEnvironment(t *testing.T) {
	t.Setenv("LIBREDASH_ADDR", ":19090")
	if got, want := healthcheckURL(&rootOptions{}), "http://127.0.0.1:19090/readyz"; got != want {
		t.Fatalf("healthcheck URL = %q, want %q", got, want)
	}

	t.Setenv("LIBREDASH_HEALTHCHECK_URL", "http://127.0.0.1:19091/custom-ready")
	if got, want := healthcheckURL(&rootOptions{}), "http://127.0.0.1:19091/custom-ready"; got != want {
		t.Fatalf("explicit healthcheck URL = %q, want %q", got, want)
	}
}

func TestHealthcheckURLForListenAddrUsesLoopbackForWildcardHosts(t *testing.T) {
	for _, addr := range []string{":18080", "0.0.0.0:18080", "[::]:18080", "18080"} {
		if got, want := healthcheckURLForListenAddr(addr), "http://127.0.0.1:18080/readyz"; got != want {
			t.Fatalf("healthcheck URL for %q = %q, want %q", addr, got, want)
		}
	}
	if got, want := healthcheckURLForListenAddr("localhost:19080"), "http://localhost:19080/readyz"; got != want {
		t.Fatalf("healthcheck URL = %q, want %q", got, want)
	}
}
