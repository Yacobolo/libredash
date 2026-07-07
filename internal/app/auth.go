package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/access/httpauth"
	oidcauth "github.com/Yacobolo/libredash/internal/access/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type principalContextKey struct{}
type apiCredentialContextKey struct{}

const csrfCookieName = "ld_csrf"
const oidcStateCookieName = "ld_oidc_state"
const oidcStateMaxAge = 10 * time.Minute
const oidcStateClockSkew = time.Minute

var (
	errUnauthorized = errors.New("unauthorized")
	errForbidden    = errors.New("forbidden")
)

type Principal struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	DevBypass   bool   `json:"-"`
}

type oidcClient interface {
	AuthCodeURL(state, nonce string) string
	Authenticate(ctx context.Context, code, expectedNonce string) (oidcauth.Claims, error)
}

var authRandomReader io.Reader = rand.Reader
var authNow = time.Now

type sessionManager interface {
	CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error)
	PrincipalForToken(ctx context.Context, token string) (access.Principal, error)
	DeleteSession(ctx context.Context, token string) error
}

type localCredentialManager interface {
	VerifyLocalPassword(ctx context.Context, email, password string) (access.Principal, access.LocalCredential, error)
	ChangeLocalPassword(ctx context.Context, principalID, currentPassword, newPassword string) (access.LocalCredential, error)
	LocalCredential(ctx context.Context, principalID string) (access.LocalCredential, error)
}

type disabledCredentialResolver interface {
	DisabledPrincipalForAPIToken(ctx context.Context, token string) (principalID, tokenID string, err error)
	DisabledPrincipalForSessionToken(ctx context.Context, token string) (principalID, sessionID string, err error)
}

type Auth struct {
	repo         access.Repository
	sessions     sessionManager
	workspaceID  string
	devBypass    bool
	apiTokenOnly bool
	localAuth    bool
	enabled      bool
	configured   bool
	azureTenant  string
	cookieSecure bool
	csrf         func(http.Handler) http.Handler
	oidcRegistry *oidcauth.Registry
	oidcOverride map[string]oidcClient
	stateKey     []byte
}

type AuthConfig struct {
	DevBypass       bool
	APITokenOnly    bool
	LocalAuth       bool
	AzureClientID   string
	AzureSecret     string
	AzureCallback   string
	AzureTenant     string
	CSRFKey         string
	CookieSecure    bool
	BootstrapTenant string
	OIDCProviders   []oidcauth.Config
}

func NewAuth(repo access.Repository, workspaceID string, cfg AuthConfig) *Auth {
	auth := &Auth{
		repo:         repo,
		sessions:     repo,
		workspaceID:  workspaceID,
		devBypass:    cfg.DevBypass,
		apiTokenOnly: cfg.APITokenOnly,
		localAuth:    cfg.LocalAuth,
		azureTenant:  cfg.AzureTenant,
		cookieSecure: cfg.CookieSecure,
	}
	providers := append([]oidcauth.Config(nil), cfg.OIDCProviders...)
	if cfg.AzureClientID != "" && cfg.AzureSecret != "" && cfg.AzureCallback != "" && !hasOIDCProvider(providers, "azureadv2") {
		providers = append(providers, oidcauth.AzureProviderConfig(cfg.AzureClientID, cfg.AzureSecret, cfg.AzureCallback, cfg.AzureTenant))
	}
	if registry, err := oidcauth.NewRegistry(providers); err == nil {
		auth.oidcRegistry = registry
		auth.configured = registry.Configured()
	}
	auth.csrf = csrf.Protect(
		csrfKey(cfg.CSRFKey),
		csrf.CookieName(csrfCookieName),
		csrf.Path("/"),
		csrf.Secure(cfg.CookieSecure),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if wantsJSON(r) {
				writeJSONError(w, csrf.FailureReason(r), http.StatusForbidden)
				return
			}
			http.Error(w, csrf.FailureReason(r).Error(), http.StatusForbidden)
		})),
	)
	auth.stateKey = derivedSecret(cfg.CSRFKey, "oidc-state")
	auth.enabled = true
	return auth
}

func hasOIDCProvider(providers []oidcauth.Config, id string) bool {
	id = oidcauth.ProviderID(id)
	for _, provider := range providers {
		if oidcauth.ProviderID(provider.ID) == id {
			return true
		}
	}
	return false
}

func (a *Auth) Enabled() bool {
	return a != nil && a.enabled
}

func (a *Auth) oidcClient(ctx context.Context, provider string) (oidcClient, oidcauth.Config, error) {
	provider = oidcauth.ProviderID(provider)
	if a.oidcOverride != nil {
		if client := a.oidcOverride[provider]; client != nil {
			var cfg oidcauth.Config
			if a.oidcRegistry != nil {
				cfg, _ = a.oidcRegistry.Config(provider)
			}
			cfg.ID = provider
			return client, cfg, nil
		}
	}
	if a.oidcRegistry == nil {
		return nil, oidcauth.Config{}, errors.New("oidc registry is not configured")
	}
	client, cfg, err := a.oidcRegistry.Client(ctx, provider)
	return client, cfg, err
}

func (a *Auth) Begin(w http.ResponseWriter, r *http.Request) {
	if !a.configured && !a.devBypass {
		http.Error(w, "OIDC auth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "azureadv2"
	}
	client, _, err := a.oidcClient(r.Context(), provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	state, err := randomAuthValue()
	if err != nil {
		http.Error(w, "secure randomness unavailable", http.StatusInternalServerError)
		return
	}
	nonce, err := randomAuthValue()
	if err != nil {
		http.Error(w, "secure randomness unavailable", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, a.oidcStateCookie(state, nonce))
	http.Redirect(w, r, client.AuthCodeURL(state, nonce), http.StatusFound)
}

func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	if !a.configured && !a.devBypass {
		http.Error(w, "OIDC auth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "azureadv2"
	}
	state, nonce, err := a.consumeOIDCState(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if r.URL.Query().Get("state") != state {
		http.Error(w, "invalid oidc state", http.StatusUnauthorized)
		return
	}
	client, providerConfig, err := a.oidcClient(r.Context(), provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	claims, err := client.Authenticate(r.Context(), r.URL.Query().Get("code"), nonce)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	email := oidcEmail(claims)
	issuer := firstNonEmpty(claims.Issuer, providerConfig.IssuerURL, provider)
	principal, err := a.repo.ResolveExternalPrincipal(r.Context(), access.ExternalIdentityInput{
		Provider:    "oidc",
		TenantID:    issuer,
		Subject:     stableSubject(claims.Subject, email),
		Email:       email,
		DisplayName: oidcDisplayName(claims),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, err := a.sessions.CreateSession(r.Context(), principal.ID, 8*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recordAccessAudit(r, a.repo, "session.created", principal.ID, "", "session", "", "", "success", map[string]any{"provider": provider})
	recordAccessAudit(r, a.repo, "sign_in", principal.ID, "", "principal", principal.ID, "", "success", map[string]any{"provider": provider})
	http.SetCookie(w, a.sessionCookie(token, time.Now().Add(8*time.Hour)))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("ld_session"); err == nil {
		principal, _ := a.sessions.PrincipalForToken(r.Context(), cookie.Value)
		_ = a.sessions.DeleteSession(r.Context(), cookie.Value)
		recordAccessAudit(r, a.repo, "session.revoked", principal.ID, "", "session", "", "", "success", nil)
		recordAccessAudit(r, a.repo, "sign_out", principal.ID, "", "principal", principal.ID, "", "success", nil)
	}
	http.SetCookie(w, &http.Cookie{Name: "ld_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cookieSecure})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) LocalLogin(w http.ResponseWriter, r *http.Request) {
	if !a.localAuth {
		http.Error(w, "local auth is not enabled", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := access.NormalizeEmail(firstNonEmpty(r.Form.Get("email"), r.Form.Get("username")))
	password := r.Form.Get("password")
	local, ok := a.repo.(localCredentialManager)
	if !ok {
		http.Error(w, "local auth repository is not configured", http.StatusServiceUnavailable)
		return
	}
	principal, credential, err := local.VerifyLocalPassword(r.Context(), email, password)
	if err != nil {
		recordAccessAudit(r, a.repo, "sign_in", "", "", "principal", "", "", "denied", map[string]any{"provider": "local", "email": email})
		if wantsJSON(r) {
			writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
			return
		}
		http.Error(w, errUnauthorized.Error(), http.StatusUnauthorized)
		return
	}
	token, err := a.sessions.CreateSession(r.Context(), principal.ID, 8*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recordAccessAudit(r, a.repo, "session.created", principal.ID, "", "session", "", "", "success", map[string]any{"provider": "local"})
	recordAccessAudit(r, a.repo, "sign_in", principal.ID, "", "principal", principal.ID, "", "success", map[string]any{"provider": "local"})
	http.SetCookie(w, a.sessionCookie(token, time.Now().Add(8*time.Hour)))
	if credential.MustChangePassword {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) LocalPassword(w http.ResponseWriter, r *http.Request) {
	if !a.localAuth {
		http.Error(w, "local auth is not enabled", http.StatusServiceUnavailable)
		return
	}
	principal, _, ok := a.authenticate(r)
	if !ok {
		if wantsJSON(r) {
			writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
			return
		}
		http.Error(w, errUnauthorized.Error(), http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	local, ok := a.repo.(localCredentialManager)
	if !ok {
		http.Error(w, "local auth repository is not configured", http.StatusServiceUnavailable)
		return
	}
	if _, err := local.ChangeLocalPassword(r.Context(), principal.ID, firstNonEmpty(r.Form.Get("currentPassword"), r.Form.Get("current_password")), firstNonEmpty(r.Form.Get("newPassword"), r.Form.Get("new_password"))); err != nil {
		if wantsJSON(r) {
			writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
			return
		}
		http.Error(w, errUnauthorized.Error(), http.StatusUnauthorized)
		return
	}
	recordAccessAudit(r, a.repo, "password.changed", principal.ID, "", "principal", principal.ID, "", "success", map[string]any{"provider": "local"})
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "changed"})
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) Principal(r *http.Request) (Principal, bool) {
	if value, ok := r.Context().Value(principalContextKey{}).(Principal); ok {
		return value, true
	}
	return Principal{}, false
}

func (a *Auth) APICredential(r *http.Request) (access.APICredential, bool) {
	if value, ok := r.Context().Value(apiCredentialContextKey{}).(access.APICredential); ok {
		return value, true
	}
	return access.APICredential{}, false
}

func (a *Auth) Middleware(privilege access.Privilege, next http.Handler) http.Handler {
	return a.MiddlewareWithObjectResolver(privilege, nil, next)
}

func (a *Auth) MiddlewareWithObjectResolver(privilege access.Privilege, objectResolver httpauth.ObjectResolver, next http.Handler) http.Handler {
	if !a.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(access.WithAuthorizationCache(r.Context()))
		principal, credential, ok := a.authenticate(r)
		if !ok {
			if a.apiTokenOnly {
				writeBearerChallenge(w, r)
				return
			}
			if wantsJSON(r) {
				writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, a.defaultLoginRedirect(), http.StatusFound)
			return
		}
		if a.mustChangeLocalPassword(r, principal.ID) {
			writeAuthError(w, r, errForbidden, http.StatusForbidden)
			return
		}
		if privilege != "" {
			workspaceID := a.privilegeWorkspaceID(r)
			if credential != nil && !apiTokenAllows((*credential).Token, workspaceID, privilege) {
				writeAuthError(w, r, errForbidden, http.StatusForbidden)
				return
			}
			objects := httpauth.ObjectsForRequest(privilege, r, workspaceID)
			if objectResolver != nil && privilege != access.PrivilegeManagePlatform {
				if resolved := objectResolver(r, workspaceID); len(resolved) > 0 {
					objects = resolved
				}
			}
			decision, err := a.repo.AuthorizeAny(r.Context(), principal.ID, privilege, objects)
			if err != nil {
				writeAuthError(w, r, err, http.StatusInternalServerError)
				return
			}
			if !decision.Allowed && httpauth.CanDeferDataAuth(privilege) {
				useDecision, err := a.repo.Authorize(r.Context(), principal.ID, access.PrivilegeUseWorkspace, httpauth.ObjectForWorkspace(workspaceID))
				if err != nil {
					writeAuthError(w, r, err, http.StatusInternalServerError)
					return
				}
				if useDecision.Allowed {
					decision.Allowed = true
				}
			}
			if !decision.Allowed && httpauth.CanDeferDataAuth(privilege) {
				viewDecision, err := a.repo.AuthorizeAny(r.Context(), principal.ID, access.PrivilegeViewItem, objects)
				if err != nil {
					writeAuthError(w, r, err, http.StatusInternalServerError)
					return
				}
				if viewDecision.Allowed {
					decision.Allowed = true
				}
			}
			if !decision.Allowed && httpauth.RouteCanDeferGrantManagement(privilege, r) {
				useDecision, err := a.repo.Authorize(r.Context(), principal.ID, access.PrivilegeUseWorkspace, httpauth.ObjectForWorkspace(workspaceID))
				if err != nil {
					writeAuthError(w, r, err, http.StatusInternalServerError)
					return
				}
				if useDecision.Allowed {
					decision.Allowed = true
				}
			}
			if !decision.Allowed {
				writeAuthError(w, r, errForbidden, http.StatusForbidden)
				return
			}
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		if credential != nil {
			ctx = context.WithValue(ctx, apiCredentialContextKey{}, *credential)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) defaultLoginRedirect() string {
	if a.localAuth {
		return "/login"
	}
	return "/auth/azureadv2"
}

func (a *Auth) mustChangeLocalPassword(r *http.Request, principalID string) bool {
	if !a.localAuth || r.URL.Path == "/auth/local/password" || r.URL.Path == "/auth/logout" {
		return false
	}
	local, ok := a.repo.(localCredentialManager)
	if !ok {
		return false
	}
	credential, err := local.LocalCredential(r.Context(), principalID)
	return err == nil && credential.MustChangePassword
}

func writeBearerChallenge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="libredash"`)
	if wantsJSON(r) {
		writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
		return
	}
	http.Error(w, "API bearer token required", http.StatusUnauthorized)
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if wantsJSON(r) {
		writeJSONError(w, err, status)
		return
	}
	http.Error(w, err.Error(), status)
}

func recordAccessAudit(r *http.Request, repo access.Repository, action, principalID, workspaceID, targetType, targetID string, privilege access.Privilege, status string, metadata map[string]any) {
	if repo == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	bytes, _ := json.Marshal(metadata)
	_ = repo.RecordAuditEvent(r.Context(), access.AuditEventInput{
		WorkspaceID:   workspaceID,
		PrincipalID:   principalID,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		Privilege:     privilege,
		Status:        status,
		RequestID:     firstNonEmpty(r.Header.Get("X-Request-Id"), r.Header.Get("X-Request-ID")),
		CorrelationID: firstNonEmpty(r.Header.Get("X-Correlation-Id"), r.Header.Get("X-Correlation-ID"), r.Header.Get("X-Request-Id"), r.Header.Get("X-Request-ID")),
		MetadataJSON:  string(bytes),
	})
}

func (a *Auth) privilegeWorkspaceID(r *http.Request) string {
	if workspaceID := strings.TrimSpace(chi.URLParam(r, "workspace")); workspaceID != "" {
		return workspaceID
	}
	if workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace")); workspaceID != "" {
		if workspaceID == "platform" && r.URL.Path == "/updates" {
			return ""
		}
		return workspaceID
	}
	return a.workspaceID
}

func (a *Auth) CSRFMiddleware(next http.Handler) http.Handler {
	if a == nil || a.csrf == nil {
		return next
	}
	protected := a.csrf(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearerToken(r) != "" && !hasSessionCookie(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !a.cookieSecure && r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			r = csrf.PlaintextHTTPRequest(r)
		}
		protected.ServeHTTP(w, r)
	})
}

func (a *Auth) authenticate(r *http.Request) (Principal, *access.APICredential, bool) {
	if a.devBypass {
		return localDeveloperPrincipal(), nil, true
	}
	if token := bearerToken(r); token != "" {
		credential, err := a.repo.CredentialForAPIToken(r.Context(), token)
		if err == nil {
			principal := credential.Principal
			return Principal{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}, &credential, true
		}
		a.auditDisabledCredentialFailure(r, "api_token", token)
		return Principal{}, nil, false
	}
	if a.apiTokenOnly {
		return Principal{}, nil, false
	}
	cookie, err := r.Cookie("ld_session")
	if err != nil || cookie.Value == "" {
		return Principal{}, nil, false
	}
	principal, err := a.sessions.PrincipalForToken(r.Context(), cookie.Value)
	if err != nil {
		a.auditDisabledCredentialFailure(r, "session", cookie.Value)
		return Principal{}, nil, false
	}
	return Principal{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}, nil, true
}

func hasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie("ld_session")
	return err == nil && strings.TrimSpace(cookie.Value) != ""
}

func (a *Auth) auditDisabledCredentialFailure(r *http.Request, credentialType, secret string) {
	resolver, ok := a.repo.(disabledCredentialResolver)
	if !ok || strings.TrimSpace(secret) == "" {
		return
	}
	var principalID, targetID string
	var err error
	switch credentialType {
	case "api_token":
		principalID, targetID, err = resolver.DisabledPrincipalForAPIToken(r.Context(), secret)
	case "session":
		principalID, targetID, err = resolver.DisabledPrincipalForSessionToken(r.Context(), secret)
	default:
		return
	}
	if err != nil || principalID == "" {
		return
	}
	recordAccessAudit(r, a.repo, "credential.denied", principalID, "", credentialType, targetID, "", "denied", map[string]any{"reason": "principal_disabled"})
}

func apiTokenAllows(token access.APIToken, workspaceID string, privilege access.Privilege) bool {
	if token.WorkspaceID != "" && token.WorkspaceID != workspaceID {
		return false
	}
	if token.Privileges == nil {
		return true
	}
	for _, allowed := range token.Privileges {
		if allowed == privilege {
			return true
		}
	}
	return false
}

func (a *Auth) sessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     "ld_session",
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.cookieSecure,
	}
}

func wantsJSON(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") || strings.Contains(r.Header.Get("Accept"), "application/json")
}

func oidcDisplayName(claims oidcauth.Claims) string {
	if claims.Name != "" {
		return claims.Name
	}
	if claims.PreferredUsername != "" {
		return claims.PreferredUsername
	}
	return claims.Email
}

func oidcEmail(claims oidcauth.Claims) string {
	if claims.Email != "" {
		return claims.Email
	}
	return claims.PreferredUsername
}

func stableSubject(subject, fallback string) string {
	if subject != "" {
		return subject
	}
	return fallback
}

func bearerToken(r *http.Request) string {
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return fields[1]
}

func csrfKey(value string) []byte {
	if len(value) >= 32 {
		return []byte(value)[:32]
	}
	seed := access.PrincipalIDForEmail("csrf:" + value)
	key := make([]byte, 32)
	copy(key, []byte(seed))
	return key
}

func (a *Auth) oidcStateCookie(state, nonce string) *http.Cookie {
	return &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    a.encodeOIDCState(state, nonce),
		Path:     "/",
		MaxAge:   int(oidcStateMaxAge / time.Second),
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func (a *Auth) consumeOIDCState(w http.ResponseWriter, r *http.Request) (string, string, error) {
	cookie, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		return "", "", err
	}
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cookieSecure})
	return a.decodeOIDCState(cookie.Value)
}

func (a *Auth) encodeOIDCState(state, nonce string) string {
	return a.encodeOIDCStateAt(state, nonce, authNow())
}

func (a *Auth) encodeOIDCStateAt(state, nonce string, issuedAt time.Time) string {
	issuedUnix := strconv.FormatInt(issuedAt.UTC().Unix(), 10)
	message := state + "|" + nonce + "|" + issuedUnix
	mac := hmac.New(sha256.New, a.stateKey)
	mac.Write([]byte(message))
	return state + "." + nonce + "." + issuedUnix + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) decodeOIDCState(value string) (string, string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return "", "", errors.New("invalid oidc state cookie")
	}
	issuedUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", errors.New("invalid oidc state cookie timestamp")
	}
	issuedAt := time.Unix(issuedUnix, 0).UTC()
	now := authNow().UTC()
	if issuedAt.After(now.Add(oidcStateClockSkew)) {
		return "", "", errors.New("oidc state cookie is not yet valid")
	}
	if !issuedAt.Add(oidcStateMaxAge).After(now) {
		return "", "", errors.New("oidc state cookie expired")
	}
	expected := a.encodeOIDCStateAt(parts[0], parts[1], issuedAt)
	if !hmac.Equal([]byte(value), []byte(expected)) {
		return "", "", errors.New("invalid oidc state cookie signature")
	}
	return parts[0], parts[1], nil
}

func derivedSecret(secret, purpose string) []byte {
	base := csrfKey(secret)
	sum := sha256.Sum256(append([]byte("libredash:"+purpose+":"), base...))
	return sum[:]
}

func randomAuthValue() (string, error) {
	var b [32]byte
	if _, err := io.ReadFull(authRandomReader, b[:]); err != nil {
		return "", fmt.Errorf("read secure random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func setAuthRandomReaderForTest(reader io.Reader) func() {
	previous := authRandomReader
	authRandomReader = reader
	return func() {
		authRandomReader = previous
	}
}

func setAuthNowForTest(now time.Time) func() {
	previous := authNow
	authNow = func() time.Time { return now }
	return func() {
		authNow = previous
	}
}
