package app

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	oidcauth "github.com/Yacobolo/libredash/internal/access/oidc"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
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
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{
		Store:              store,
		Auth:               auth,
		ArtifactDir:        t.TempDir(),
		DefaultWorkspaceID: "test",
		RateLimits:         RateLimitConfig{Enabled: true, APILimit: 1, APIWindow: time.Minute},
	})
	handler := server.Routes()

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/publishes?workspace=test", nil)
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

func TestDevBypassStillUsesGrantPrivileges(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	repo := accesssqlite.NewRepository(store.SQLDB())
	auth := NewAuth(repo, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, ArtifactDir: t.TempDir(), DefaultWorkspaceID: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/publishes?workspace=test", nil)
	req.Header.Set("Authorization", "Bearer dev")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unseeded dev status = %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	if err := SeedLocalDeveloperPlatformAdmin(ctx, repo); err != nil {
		t.Fatalf("seed local developer: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test/publishes?workspace=test", nil)
	req.Header.Set("Authorization", "Bearer dev")
	req.Header.Set("Accept", "application/json")
	rec = httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("seeded dev status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestUpdatesRateLimitAllowsOrdinaryReconnects(t *testing.T) {
	auth := testAuth(testStore(t), "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(fakeMetrics{}, Options{
		Auth:       auth,
		RateLimits: RateLimitConfig{Enabled: true, UpdatesLimit: 2, UpdatesWindow: time.Minute},
	})
	handler := server.Routes()

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/workspaces/test-workspace/updates?dashboard=executive-sales&page=overview", nil)
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

func TestAuthBeginCreatesOIDCStateCookieAndRedirect(t *testing.T) {
	auth := testAuth(testStore(t), "test", AuthConfig{
		DevBypass:    true,
		CSRFKey:      "0123456789abcdef0123456789abcdef",
		CookieSecure: true,
	})
	auth.configured = true
	auth.oidcOverride = map[string]oidcClient{"azureadv2": fakeOIDCClient{authURL: "https://issuer.example/authorize"}}

	req := httptest.NewRequest(http.MethodGet, "/auth/azureadv2", nil)
	rec := httptest.NewRecorder()
	auth.Begin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "https://issuer.example/authorize?") {
		t.Fatalf("Location = %q", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	state := parsed.Query().Get("state")
	nonce := parsed.Query().Get("nonce")
	if state == "" || nonce == "" {
		t.Fatalf("redirect missing state/nonce: %s", location)
	}
	cookie := oidcStateCookieForTest(rec.Result().Cookies())
	if cookie == nil {
		t.Fatalf("%s cookie missing", oidcStateCookieName)
	}
	if cookie.HttpOnly != true || cookie.Secure != true || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 10*60 {
		t.Fatalf("state cookie options = %#v", cookie)
	}
	cookieState, cookieNonce, err := auth.decodeOIDCState(cookie.Value)
	if err != nil {
		t.Fatalf("decode state cookie: %v", err)
	}
	if cookieState != state || cookieNonce != nonce {
		t.Fatalf("cookie state/nonce = %q/%q, redirect = %q/%q", cookieState, cookieNonce, state, nonce)
	}
}

func TestProductionConfigFailsBeforeAuthStoreSetupWithoutCSRFKey(t *testing.T) {
	cfg := config.Config{Production: true, APITokenOnlyAuth: true}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected production auth validation to fail")
	}
}

func TestAuthCallbackRejectsInvalidOIDCState(t *testing.T) {
	auth := testAuth(testStore(t), "test", AuthConfig{
		DevBypass: true,
		CSRFKey:   "0123456789abcdef0123456789abcdef",
	})
	auth.configured = true
	auth.oidcOverride = map[string]oidcClient{"azureadv2": fakeOIDCClient{}}
	req := httptest.NewRequest(http.MethodGet, "/auth/azureadv2/callback?state=wrong&code=code", nil)
	req.AddCookie(auth.oidcStateCookie("right", "nonce"))
	rec := httptest.NewRecorder()

	auth.Callback(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthCallbackCreatesSessionAndAuditEvents(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{
		DevBypass: true,
		CSRFKey:   "0123456789abcdef0123456789abcdef",
	})
	auth.configured = true
	auth.oidcOverride = map[string]oidcClient{"azureadv2": fakeOIDCClient{claims: oidcauth.Claims{Issuer: "https://issuer.example", Subject: "subject-1", Email: "user@example.com", Name: "User Example"}}}
	req := httptest.NewRequest(http.MethodGet, "/auth/azureadv2/callback?state=state&code=code", nil)
	req.AddCookie(auth.oidcStateCookie("state", "nonce"))
	rec := httptest.NewRecorder()

	auth.Callback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 body=%s", rec.Code, rec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "ld_session" && cookie.Value != "" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("session cookie missing")
	}
	principal, err := accesssqlite.NewRepository(store.SQLDB()).PrincipalForToken(context.Background(), sessionCookie.Value)
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	if principal.Email != "user@example.com" || principal.DisplayName != "User Example" {
		t.Fatalf("principal = %#v", principal)
	}
	events, err := accesssqlite.NewRepository(store.SQLDB()).ListAuditEvents(context.Background(), access.AuditEventFilter{Action: "sign_in"})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 || events[0].PrincipalID != principal.ID {
		t.Fatalf("sign_in events = %#v, principal %q", events, principal.ID)
	}
}

func TestAuthCallbackUsesOIDCIssuerAndSubjectAsStableIdentity(t *testing.T) {
	store := testStore(t)
	auth := testAuth(store, "test", AuthConfig{
		DevBypass: true,
		CSRFKey:   "0123456789abcdef0123456789abcdef",
	})
	auth.configured = true
	repo := accesssqlite.NewRepository(store.SQLDB())

	auth.oidcOverride = map[string]oidcClient{"azureadv2": fakeOIDCClient{claims: oidcauth.Claims{Issuer: "https://issuer.example", Subject: "subject-1", Email: "first@example.com", Name: "First"}}}
	req := httptest.NewRequest(http.MethodGet, "/auth/azureadv2/callback?state=state&code=code", nil)
	req.AddCookie(auth.oidcStateCookie("state", "nonce"))
	rec := httptest.NewRecorder()
	auth.Callback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("first callback status = %d, want 302 body=%s", rec.Code, rec.Body.String())
	}
	first := principalFromSessionCookie(t, repo, rec.Result().Cookies())

	auth.oidcOverride = map[string]oidcClient{"azureadv2": fakeOIDCClient{claims: oidcauth.Claims{Issuer: "https://issuer.example", Subject: "subject-1", Email: "second@example.com", Name: "Second"}}}
	req = httptest.NewRequest(http.MethodGet, "/auth/azureadv2/callback?state=state&code=code", nil)
	req.AddCookie(auth.oidcStateCookie("state", "nonce"))
	rec = httptest.NewRecorder()
	auth.Callback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("second callback status = %d, want 302 body=%s", rec.Code, rec.Body.String())
	}
	second := principalFromSessionCookie(t, repo, rec.Result().Cookies())

	if second.ID != first.ID {
		t.Fatalf("principal changed after email update: first=%q second=%q", first.ID, second.ID)
	}
	if second.Email != "second@example.com" || second.DisplayName != "Second" {
		t.Fatalf("updated principal = %#v, want latest OIDC metadata", second)
	}
}

func TestAuthAuditsDisabledPrincipalCredentialFailures(t *testing.T) {
	store := testStore(t)
	repo := accesssqlite.NewRepository(store.SQLDB())
	ctx := context.Background()
	user, err := repo.UpsertSCIMUser(ctx, access.SCIMUserInput{
		ExternalID:  "disabled-auth-user",
		UserName:    "disabled-auth@example.com",
		Email:       "disabled-auth@example.com",
		DisplayName: "Disabled Auth",
		Active:      true,
	})
	if err != nil {
		t.Fatalf("create SCIM user: %v", err)
	}
	sessionSecret, err := repo.CreateSession(ctx, user.Principal.ID, time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	apiSecret, _, err := repo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{PrincipalID: user.Principal.ID, Name: "disabled-auth-token"})
	if err != nil {
		t.Fatalf("create API token: %v", err)
	}
	if _, err := repo.DisableSCIMUser(ctx, user.Principal.ID); err != nil {
		t.Fatalf("disable SCIM user: %v", err)
	}
	auth := NewAuth(repo, "test", AuthConfig{CSRFKey: "0123456789abcdef0123456789abcdef"})

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/test", nil)
	apiReq.Header.Set("Authorization", "Bearer "+apiSecret)
	apiReq.Header.Set("X-Request-ID", "disabled_api_req")
	if _, _, ok := auth.authenticate(apiReq); ok {
		t.Fatal("disabled API token authenticated")
	}
	sessionReq := httptest.NewRequest(http.MethodGet, "/workspaces/test", nil)
	sessionReq.AddCookie(&http.Cookie{Name: "ld_session", Value: sessionSecret})
	sessionReq.Header.Set("X-Request-ID", "disabled_session_req")
	if _, _, ok := auth.authenticate(sessionReq); ok {
		t.Fatal("disabled session authenticated")
	}
	if _, err := repo.CredentialForAPIToken(ctx, apiSecret); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("disabled api credential err = %v, want sql.ErrNoRows", err)
	}

	events, err := repo.ListAuditEvents(ctx, access.AuditEventFilter{Action: "credential.denied"})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("credential denied events = %d, want 2: %#v", len(events), events)
	}
	byRequest := map[string]access.AuditEvent{}
	for _, event := range events {
		byRequest[event.RequestID] = event
	}
	for requestID, targetType := range map[string]string{"disabled_api_req": "api_token", "disabled_session_req": "session"} {
		event, ok := byRequest[requestID]
		if !ok {
			t.Fatalf("missing audit event for %s: %#v", requestID, events)
		}
		if event.PrincipalID != user.Principal.ID || event.Status != "denied" || event.TargetType != targetType || !strings.Contains(event.MetadataJSON, "principal_disabled") {
			t.Fatalf("audit event for %s = %#v", requestID, event)
		}
	}
}

type fakeOIDCClient struct {
	authURL string
	claims  oidcauth.Claims
}

func (c fakeOIDCClient) AuthCodeURL(state, nonce string) string {
	base := c.authURL
	if base == "" {
		base = "https://issuer.example/authorize"
	}
	values := url.Values{}
	values.Set("state", state)
	values.Set("nonce", nonce)
	return base + "?" + values.Encode()
}

func (c fakeOIDCClient) Authenticate(_ context.Context, _ string, expectedNonce string) (oidcauth.Claims, error) {
	if expectedNonce != "nonce" {
		return oidcauth.Claims{}, errUnauthorized
	}
	return c.claims, nil
}

func oidcStateCookieForTest(cookies []*http.Cookie) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == oidcStateCookieName {
			return cookie
		}
	}
	return nil
}

func principalFromSessionCookie(t *testing.T, repo *accesssqlite.Repository, cookies []*http.Cookie) access.Principal {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == "ld_session" && cookie.Value != "" {
			principal, err := repo.PrincipalForToken(context.Background(), cookie.Value)
			if err != nil {
				t.Fatalf("resolve session: %v", err)
			}
			return principal
		}
	}
	t.Fatal("session cookie missing")
	return access.Principal{}
}
