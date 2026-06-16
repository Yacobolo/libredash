package app

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/config"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth/gothic"
)

func TestAuthRouteRateLimit(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{
		RateLimits: RateLimitConfig{Enabled: true, AuthLimit: 1, AuthWindow: time.Minute},
	})
	handler := server.Routes()

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/azureadv2", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i == 0 && rec.Code == http.StatusTooManyRequests {
			t.Fatal("first auth request was unexpectedly rate limited")
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second auth status = %d, want %d", rec.Code, http.StatusTooManyRequests)
		}
	}
}

func TestDeploymentAPIRateLimitPreservesAuth(t *testing.T) {
	store := testStore(t)
	auth := NewAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{
		Store:              store,
		Auth:               auth,
		ArtifactDir:        t.TempDir(),
		DefaultWorkspaceID: "test",
		RateLimits:         RateLimitConfig{Enabled: true, APILimit: 1, APIWindow: time.Minute},
	})
	handler := server.Routes()

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/deployments?workspace=test", nil)
		req.RemoteAddr = "192.0.2.20:1234"
		req.Header.Set("Authorization", "Bearer dev")
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i == 0 && rec.Code != http.StatusOK {
			t.Fatalf("first API status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if i == 1 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second API status = %d, want %d", rec.Code, http.StatusTooManyRequests)
		}
	}
}

func TestUpdatesRateLimitAllowsOrdinaryReconnects(t *testing.T) {
	auth := NewAuth(testStore(t), "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{
		Auth:       auth,
		RateLimits: RateLimitConfig{Enabled: true, UpdatesLimit: 2, UpdatesWindow: time.Minute},
	})
	handler := server.Routes()

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?dashboard=executive-sales&page=overview", nil)
		req.RemoteAddr = "192.0.2.30:1234"
		req.Header.Set("Authorization", "Bearer dev")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("reconnect %d was unexpectedly rate limited", i+1)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{
		SecurityHeaders: SecurityHeaders(true),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	headers := rec.Result().Header
	for name, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Permissions-Policy":        "camera=(), microphone=(), geolocation=()",
		"X-Frame-Options":           "SAMEORIGIN",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	} {
		if got := headers.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestSecurityHeadersOmitHSTSWhenDisabled(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{
		SecurityHeaders: SecurityHeaders(false),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if got := rec.Result().Header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want empty", got)
	}
}

func TestRequestLoggerDoesNotLogSensitiveHeaders(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := NewWithOptions(fakeMetrics{}, Options{
		RequestLogging: true,
		Logger:         logger,
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.AddCookie(&http.Cookie{Name: "ld_session", Value: "secret-session"})
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	logged := buf.String()
	for _, want := range []string{"method=GET", "path=/", "status=200", "duration="} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log %q missing %q", logged, want)
		}
	}
	for _, secret := range []string{"secret-token", "secret-session", "Authorization", "Cookie"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("log %q contains sensitive value %q", logged, secret)
		}
	}
}

func TestNewAuthConfiguresGothCookieStore(t *testing.T) {
	_ = NewAuth(testStore(t), "test", AuthConfig{
		DevBypass:    true,
		CSRFKey:      "0123456789abcdef0123456789abcdef",
		CookieSecure: true,
	})
	store, ok := gothic.Store.(*sessions.CookieStore)
	if !ok {
		t.Fatalf("gothic.Store = %T, want *sessions.CookieStore", gothic.Store)
	}
	if !store.Options.HttpOnly {
		t.Fatal("goth cookie HttpOnly = false, want true")
	}
	if !store.Options.Secure {
		t.Fatal("goth cookie Secure = false, want true")
	}
	if store.Options.SameSite != http.SameSiteLaxMode {
		t.Fatalf("goth cookie SameSite = %v, want lax", store.Options.SameSite)
	}
	if store.Options.MaxAge != 10*60 {
		t.Fatalf("goth cookie MaxAge = %d, want 600", store.Options.MaxAge)
	}
}

func TestProductionConfigFailsBeforeAuthStoreSetupWithoutCSRFKey(t *testing.T) {
	before := gothic.Store
	cfg := config.Config{Production: true, APITokenOnlyAuth: true}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected production auth validation to fail")
	}
	if gothic.Store != before {
		t.Fatal("gothic.Store changed before valid production auth config")
	}
}
