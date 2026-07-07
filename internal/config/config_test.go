package config

import "testing"

func TestDuckLakeCatalogPathDefaultsOutsidePlatformDB(t *testing.T) {
	cfg := Config{HomeDir: "/var/lib/libredash"}
	if got, want := cfg.DBPath(), "/var/lib/libredash/libredash.db"; got != want {
		t.Fatalf("DBPath = %q, want %q", got, want)
	}
	if got, want := cfg.DuckLakeCatalogPath(), "/var/lib/libredash/ducklake/catalog.sqlite"; got != want {
		t.Fatalf("DuckLakeCatalogPath = %q, want %q", got, want)
	}
	if cfg.DuckLakeCatalogPath() == cfg.DBPath() {
		t.Fatal("DuckLake catalog must not default to the platform database")
	}
}

func TestDuckLakeCatalogPathHonorsExplicitPath(t *testing.T) {
	cfg := Config{HomeDir: "/var/lib/libredash", DuckLakeCatalog: "/mnt/catalog.sqlite"}
	if got, want := cfg.DuckLakeCatalogPath(), "/mnt/catalog.sqlite"; got != want {
		t.Fatalf("DuckLakeCatalogPath = %q, want %q", got, want)
	}
}

func TestValidateProductionAuthRequiresCSRFKey(t *testing.T) {
	cfg := Config{Production: true, APITokenOnlyAuth: true}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected missing CSRF key to fail production auth validation")
	}
}

func TestValidateProductionAuthRequiresMetricsBearerToken(t *testing.T) {
	cfg := Config{
		Production:       true,
		APITokenOnlyAuth: true,
		CSRFKey:          "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected missing metrics bearer token to fail production auth validation")
	}
}

func TestValidateProductionAuthRejectsDevBypass(t *testing.T) {
	cfg := Config{
		Production:         true,
		DevAuthBypass:      true,
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected production dev auth bypass to fail validation")
	}
}

func TestValidateProductionAuthAllowsGenericOIDC(t *testing.T) {
	cfg := Config{
		Production:         true,
		OIDCIssuerURL:      "https://issuer.example",
		OIDCClientID:       "client-id",
		OIDCSecret:         "client-secret",
		OIDCCallbackURL:    "https://app.example/auth/oidc/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("validate production auth: %v", err)
	}
}

func TestValidateProductionAuthAllowsLocalAuth(t *testing.T) {
	cfg := Config{
		Production:         true,
		LocalAuth:          true,
		AllowedHosts:       "libredash.example.com",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("validate production auth: %v", err)
	}
}

func TestValidateProductionAuthRejectsInsecureBrowserAuthCookies(t *testing.T) {
	cfg := Config{
		Production:         true,
		OIDCIssuerURL:      "https://issuer.example",
		OIDCClientID:       "client-id",
		OIDCSecret:         "client-secret",
		OIDCCallbackURL:    "https://app.example/auth/oidc/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
		CookieSecureRaw:    "false",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected insecure browser auth cookies to fail production validation")
	}

	cfg.APITokenOnlyAuth = true
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("api-token-only production should allow explicitly insecure browser cookies: %v", err)
	}
}

func TestValidateProductionAuthRejectsPartialGenericOIDC(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		OIDCIssuerURL:      "https://issuer.example",
		OIDCClientID:       "client-id",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected partial OIDC config to fail production auth validation")
	}
}

func TestValidateProductionAuthRejectsInsecureOIDCURLs(t *testing.T) {
	cfg := Config{
		Production:         true,
		OIDCIssuerURL:      "http://issuer.example",
		OIDCClientID:       "client-id",
		OIDCSecret:         "client-secret",
		OIDCCallbackURL:    "https://app.example/auth/oidc/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected insecure OIDC issuer URL to fail production auth validation")
	}

	cfg.OIDCIssuerURL = "https://issuer.example"
	cfg.OIDCCallbackURL = "http://app.example/auth/oidc/callback"
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected insecure OIDC callback URL to fail production auth validation")
	}
}

func TestValidateProductionAuthRejectsInsecureAzureCallbackURL(t *testing.T) {
	cfg := Config{
		Production:         true,
		AzureClientID:      "client-id",
		AzureSecret:        "client-secret",
		AzureCallbackURL:   "http://app.example/auth/azureadv2/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected insecure Azure callback URL to fail production auth validation")
	}
}

func TestValidateProductionAuthRejectsPartialAzureConfig(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		AzureClientID:      "client-id",
		AzureCallbackURL:   "https://app.example/auth/azureadv2/callback",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected partial Azure config to fail production auth validation")
	}
}

func TestValidateProductionAuthRequiresStrongSCIMBearerWhenConfigured(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		AllowedHosts:       "libredash.example.com",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		SCIMBearerToken:    "short",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected weak SCIM bearer token to fail production auth validation")
	}

	cfg.SCIMBearerToken = "0123456789abcdef0123456789abcdef"
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("strong SCIM bearer token rejected: %v", err)
	}
}

func TestValidateProductionAuthRequiresStrongMetricsBearerWhenConfigured(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		AllowedHosts:       "libredash.example.com",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "short",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected weak metrics bearer token to fail production auth validation")
	}

	cfg.MetricsBearerToken = "0123456789abcdef0123456789abcdef"
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("strong metrics bearer token rejected: %v", err)
	}
}

func TestValidateProductionAuthRequiresAllowedHostForAPITokenOnly(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected API-token-only production to require LIBREDASH_ALLOWED_HOSTS")
	}

	cfg.AllowedHosts = "libredash.example.com"
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("explicit production allowed host rejected: %v", err)
	}
}

func TestProductionAllowedHostsDerivesBrowserAuthCallbackHosts(t *testing.T) {
	cfg := Config{
		Production:         true,
		OIDCIssuerURL:      "https://issuer.example",
		OIDCClientID:       "client-id",
		OIDCSecret:         "client-secret",
		OIDCCallbackURL:    "https://app.example.com/auth/oidc/callback",
		AzureClientID:      "azure-client-id",
		AzureSecret:        "azure-client-secret",
		AzureCallbackURL:   "https://tenant.example.com/auth/azureadv2/callback",
		AllowedHosts:       "ops.example.com, *.trusted.example.com",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	hosts, err := cfg.ProductionAllowedHosts()
	if err != nil {
		t.Fatalf("production allowed hosts: %v", err)
	}
	for _, want := range []string{"ops.example.com", "*.trusted.example.com", "app.example.com", "tenant.example.com"} {
		if !containsString(hosts, want) {
			t.Fatalf("allowed hosts = %#v, missing %q", hosts, want)
		}
	}
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("derived callback hosts should satisfy production validation: %v", err)
	}
}

func TestProductionAllowedHostsRejectsInvalidEntries(t *testing.T) {
	cfg := Config{
		Production:         true,
		APITokenOnlyAuth:   true,
		AllowedHosts:       "https://app.example.com",
		CSRFKey:            "0123456789abcdef0123456789abcdef",
		MetricsBearerToken: "0123456789abcdef0123456789abcdef",
	}
	if _, err := cfg.ProductionAllowedHosts(); err == nil {
		t.Fatal("expected URL-shaped allowed host to fail validation")
	}
}

func TestOIDCScopesListSplitsWhitespaceAndCommas(t *testing.T) {
	cfg := Config{OIDCScopes: "openid profile,email\ngroups"}
	got := cfg.OIDCScopesList()
	want := []string{"openid", "profile", "email", "groups"}
	if len(got) != len(want) {
		t.Fatalf("scopes = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes = %#v, want %#v", got, want)
		}
	}
}

func TestCookieSecureDefaultsToProduction(t *testing.T) {
	secure, err := (Config{Production: true}).CookieSecure()
	if err != nil {
		t.Fatalf("cookie secure: %v", err)
	}
	if !secure {
		t.Fatal("production cookie secure default = false, want true")
	}
}

func TestProductionMiddlewareDefaults(t *testing.T) {
	cfg := Config{Production: true}
	if !cfg.RequestLoggingEnabled() {
		t.Fatal("production request logging = false, want true")
	}
	if !cfg.RateLimitingEnabled() {
		t.Fatal("production rate limiting = false, want true")
	}
	if cfg.RateLimitingUsesRealIP() {
		t.Fatal("production rate limiting trusts proxy headers by default")
	}
	if !cfg.HSTSEnabled(true) {
		t.Fatal("production HSTS with secure cookies = false, want true")
	}
	if cfg.HSTSEnabled(false) {
		t.Fatal("production HSTS without secure cookies = true, want false")
	}
}

func TestDevelopmentMiddlewareDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.RequestLoggingEnabled() {
		t.Fatal("development request logging = true, want false")
	}
	if cfg.RateLimitingEnabled() {
		t.Fatal("development rate limiting = true, want false")
	}
	if cfg.RateLimitingUsesRealIP() {
		t.Fatal("development rate limiting trusts proxy headers")
	}
	if cfg.HSTSEnabled(true) {
		t.Fatal("development HSTS = true, want false")
	}
}

func TestRateLimitingUsesRealIPRequiresProductionAndTrustProxyHeaders(t *testing.T) {
	if !(Config{Production: true, TrustProxyHeaders: true}).RateLimitingUsesRealIP() {
		t.Fatal("trusted production proxy headers should enable real IP rate limit keys")
	}
	if (Config{TrustProxyHeaders: true}).RateLimitingUsesRealIP() {
		t.Fatal("development proxy header trust should not enable real IP rate limit keys")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
