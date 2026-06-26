package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/azureadv2"
)

type principalContextKey struct{}
type apiCredentialContextKey struct{}

const csrfCookieName = "ld_csrf"

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

type Auth struct {
	repo         access.Repository
	workspaceID  string
	devBypass    bool
	apiTokenOnly bool
	enabled      bool
	configured   bool
	azureTenant  string
	cookieSecure bool
	csrf         func(http.Handler) http.Handler
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
}

func NewAuth(repo access.Repository, workspaceID string, cfg AuthConfig) *Auth {
	auth := &Auth{
		repo:         repo,
		workspaceID:  workspaceID,
		devBypass:    cfg.DevBypass,
		apiTokenOnly: cfg.APITokenOnly,
		azureTenant:  cfg.AzureTenant,
		cookieSecure: cfg.CookieSecure,
	}
	if cfg.AzureClientID != "" && cfg.AzureSecret != "" && cfg.AzureCallback != "" {
		tenant := azureadv2.TenantType(cfg.AzureTenant)
		goth.UseProviders(azureadv2.New(cfg.AzureClientID, cfg.AzureSecret, cfg.AzureCallback, azureadv2.ProviderOptions{Tenant: tenant}))
		auth.configured = true
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
	gothic.Store = gothCookieStore(cfg.CSRFKey, cfg.CookieSecure)
	auth.enabled = true
	return auth
}

func (a *Auth) Enabled() bool {
	return a != nil && a.enabled
}

func (a *Auth) Begin(w http.ResponseWriter, r *http.Request) {
	if !a.configured && !a.devBypass {
		http.Error(w, "Azure AD auth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "azureadv2"
	}
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()
	gothic.BeginAuthHandler(w, r)
}

func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	if !a.configured && !a.devBypass {
		http.Error(w, "Azure AD auth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "azureadv2"
	}
	q := r.URL.Query()
	q.Set("provider", provider)
	r.URL.RawQuery = q.Encode()
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	email := userEmail(user)
	principal, err := a.repo.ResolveExternalPrincipal(r.Context(), access.ExternalIdentityInput{
		Provider:    provider,
		TenantID:    a.azureTenant,
		Subject:     stableSubject(user.UserID, email),
		Email:       email,
		DisplayName: displayName(user),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, err := a.repo.CreateSession(r.Context(), principal.ID, 8*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, a.sessionCookie(token, time.Now().Add(8*time.Hour)))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("ld_session"); err == nil {
		_ = a.repo.DeleteSession(r.Context(), cookie.Value)
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

func (a *Auth) Middleware(permission string, next http.Handler) http.Handler {
	if !a.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, credential, ok := a.authenticate(r)
		if !ok {
			if wantsJSON(r) {
				writeJSONError(w, errUnauthorized, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/auth/azureadv2", http.StatusFound)
			return
		}
		if permission != "" {
			workspaceID := a.permissionWorkspaceID(r)
			if credential != nil && !apiTokenAllows((*credential).Token, workspaceID, permission) {
				writeAuthError(w, r, errForbidden, http.StatusForbidden)
				return
			}
			allowed, err := a.repo.HasPermission(r.Context(), workspaceID, principal.ID, permission)
			if err != nil {
				writeAuthError(w, r, err, http.StatusInternalServerError)
				return
			}
			if !allowed {
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

func (a *Auth) permissionWorkspaceID(r *http.Request) string {
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
	}
	if a.apiTokenOnly {
		return Principal{}, nil, false
	}
	cookie, err := r.Cookie("ld_session")
	if err != nil || cookie.Value == "" {
		return Principal{}, nil, false
	}
	principal, err := a.repo.PrincipalForToken(r.Context(), cookie.Value)
	if err != nil {
		if err != sql.ErrNoRows {
			return Principal{}, nil, false
		}
		return Principal{}, nil, false
	}
	return Principal{ID: principal.ID, Email: principal.Email, DisplayName: principal.DisplayName}, nil, true
}

func apiTokenAllows(token access.APIToken, workspaceID, permission string) bool {
	if token.WorkspaceID != "" && token.WorkspaceID != workspaceID {
		return false
	}
	if token.Permissions == nil {
		return true
	}
	for _, allowed := range token.Permissions {
		if allowed == permission {
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

func displayName(user goth.User) string {
	if user.Name != "" {
		return user.Name
	}
	if user.NickName != "" {
		return user.NickName
	}
	return user.Email
}

func userEmail(user goth.User) string {
	if user.Email != "" {
		return user.Email
	}
	if value, ok := user.RawData["userPrincipalName"].(string); ok {
		return value
	}
	return ""
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

func gothCookieStore(secret string, secure bool) *sessions.CookieStore {
	signingKey := derivedSecret(secret, "goth-signing")
	encryptionKey := derivedSecret(secret, "goth-encryption")
	store := sessions.NewCookieStore(signingKey, encryptionKey)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   10 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	return store
}

func derivedSecret(secret, purpose string) []byte {
	base := csrfKey(secret)
	sum := sha256.Sum256(append([]byte("libredash:"+purpose+":"), base...))
	return sum[:]
}
