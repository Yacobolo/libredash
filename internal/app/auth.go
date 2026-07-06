package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
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

type sessionManager interface {
	CreateSession(ctx context.Context, principalID string, ttl time.Duration) (string, error)
	PrincipalForToken(ctx context.Context, token string) (access.Principal, error)
	DeleteSession(ctx context.Context, token string) error
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
	state := randomAuthValue()
	nonce := randomAuthValue()
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
			if wantsJSON(r) {
				writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/auth/azureadv2", http.StatusFound)
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
		if bearerToken(r) != "" {
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
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
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
		MaxAge:   10 * 60,
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
	message := state + "|" + nonce
	mac := hmac.New(sha256.New, a.stateKey)
	mac.Write([]byte(message))
	return state + "." + nonce + "." + hex.EncodeToString(mac.Sum(nil))
}

func (a *Auth) decodeOIDCState(value string) (string, string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return "", "", errors.New("invalid oidc state cookie")
	}
	expected := a.encodeOIDCState(parts[0], parts[1])
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

func randomAuthValue() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}
