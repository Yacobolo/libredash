package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HomeDir            string `env:"LIBREDASH_HOME" envDefault:".libredash"`
	Addr               string `env:"LIBREDASH_ADDR"`
	AddrFallback       string `env:"ADDR"`
	Port               string `env:"PORT"`
	DataDir            string `env:"LIBREDASH_DATA_DIR" envDefault:".data/olist"`
	CatalogPath        string `env:"LIBREDASH_CATALOG_PATH"`
	DuckDBPath         string `env:"LIBREDASH_DUCKDB_PATH"`
	DuckDBDir          string `env:"LIBREDASH_DUCKDB_DIR"`
	DuckLakeCatalog    string `env:"LIBREDASH_DUCKLAKE_CATALOG_PATH"`
	Production         bool   `env:"LIBREDASH_PRODUCTION"`
	DevAuthBypass      bool   `env:"LIBREDASH_DEV_AUTH_BYPASS"`
	APITokenOnlyAuth   bool   `env:"LIBREDASH_API_TOKEN_ONLY_AUTH"`
	LocalAuth          bool   `env:"LIBREDASH_LOCAL_AUTH"`
	BootstrapEmail     string `env:"LIBREDASH_BOOTSTRAP_ADMIN_EMAIL"`
	AzureClientID      string `env:"LIBREDASH_AZURE_CLIENT_ID"`
	AzureSecret        string `env:"LIBREDASH_AZURE_CLIENT_SECRET"`
	AzureCallbackURL   string `env:"LIBREDASH_AZURE_CALLBACK_URL"`
	AzureTenant        string `env:"LIBREDASH_AZURE_TENANT"`
	OIDCProviderID     string `env:"LIBREDASH_OIDC_PROVIDER_ID" envDefault:"oidc"`
	OIDCIssuerURL      string `env:"LIBREDASH_OIDC_ISSUER_URL"`
	OIDCClientID       string `env:"LIBREDASH_OIDC_CLIENT_ID"`
	OIDCSecret         string `env:"LIBREDASH_OIDC_CLIENT_SECRET"`
	OIDCCallbackURL    string `env:"LIBREDASH_OIDC_CALLBACK_URL"`
	OIDCScopes         string `env:"LIBREDASH_OIDC_SCOPES"`
	SCIMBearerToken    string `env:"LIBREDASH_SCIM_BEARER_TOKEN"`
	MetricsBearerToken string `env:"LIBREDASH_METRICS_BEARER_TOKEN"`
	CSRFKey            string `env:"LIBREDASH_CSRF_KEY"`
	CookieSecureRaw    string `env:"LIBREDASH_COOKIE_SECURE"`
	TrustProxyHeaders  bool   `env:"LIBREDASH_TRUST_PROXY_HEADERS"`
	AllowedHosts       string `env:"LIBREDASH_ALLOWED_HOSTS"`
	Target             string `env:"LIBREDASH_TARGET"`
	APIToken           string `env:"LIBREDASH_API_TOKEN"`
	CLIConfig          string `env:"LIBREDASH_CLI_CONFIG"`
	AgentAPIKey        string `env:"LIBREDASH_AGENT_API_KEY"`
	AgentBaseURL       string `env:"LIBREDASH_AGENT_BASE_URL" envDefault:"https://api.openai.com/v1"`
	AgentModel         string `env:"LIBREDASH_AGENT_MODEL"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func MustLoad() Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func (c Config) ListenAddr() string {
	if c.Addr != "" {
		return c.Addr
	}
	if c.AddrFallback != "" {
		return c.AddrFallback
	}
	if c.Port != "" {
		if c.Port[0] == ':' {
			return c.Port
		}
		return ":" + c.Port
	}
	return ":8080"
}

func (c Config) DBPath() string {
	return filepath.Join(c.HomeDir, "libredash.db")
}

func (c Config) ArtifactDir() string {
	return filepath.Join(c.HomeDir, "artifacts")
}

func (c Config) RuntimeDir() string {
	return filepath.Join(c.HomeDir, "runtime")
}

func (c Config) DuckLakeDataDir() string {
	return filepath.Join(c.HomeDir, "data")
}

func (c Config) DuckLakeCatalogPath() string {
	if c.DuckLakeCatalog != "" {
		return c.DuckLakeCatalog
	}
	return filepath.Join(c.HomeDir, "ducklake", "catalog.sqlite")
}

func (c Config) DuckDBDirPath() string {
	if c.DuckDBDir != "" {
		return c.DuckDBDir
	}
	return filepath.Join(c.HomeDir, "duckdb")
}

func (c Config) ClientConfigPath() string {
	if c.CLIConfig != "" {
		return c.CLIConfig
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(c.HomeDir, "cli.json")
	}
	return filepath.Join(dir, "libredash", "cli.json")
}

func (c Config) AzureConfigured() bool {
	return c.AzureClientID != "" && c.AzureSecret != "" && c.AzureCallbackURL != ""
}

func (c Config) AzurePartiallyConfigured() bool {
	return c.AzureClientID != "" || c.AzureSecret != "" || c.AzureCallbackURL != "" || c.AzureTenant != ""
}

func (c Config) OIDCConfigured() bool {
	return c.OIDCIssuerURL != "" && c.OIDCClientID != "" && c.OIDCSecret != "" && c.OIDCCallbackURL != ""
}

func (c Config) OIDCPartiallyConfigured() bool {
	return c.OIDCIssuerURL != "" || c.OIDCClientID != "" || c.OIDCSecret != "" || c.OIDCCallbackURL != "" || c.OIDCScopes != ""
}

func (c Config) OIDCScopesList() []string {
	fields := strings.FieldsFunc(c.OIDCScopes, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			out = append(out, field)
		}
	}
	return out
}

func (c Config) AllowedHostList() ([]string, error) {
	return parseAllowedHosts(c.AllowedHosts)
}

func (c Config) ProductionAllowedHosts() ([]string, error) {
	hosts, err := c.AllowedHostList()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hosts)+2)
	add := func(host string) {
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	for _, host := range hosts {
		add(host)
	}
	for _, raw := range []string{c.OIDCCallbackURL, c.AzureCallbackURL} {
		host, err := callbackAllowedHost(raw)
		if err != nil {
			return nil, err
		}
		add(host)
	}
	return out, nil
}

func (c Config) CookieSecure() (bool, error) {
	value := strings.TrimSpace(c.CookieSecureRaw)
	if value == "" {
		return c.Production && !c.DevAuthBypass, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("LIBREDASH_COOKIE_SECURE must be a boolean: %w", err)
	}
	return parsed, nil
}

func (c Config) ValidateProductionAuth() error {
	if !c.Production {
		return nil
	}
	if c.DevAuthBypass {
		return fmt.Errorf("production serve must not enable LIBREDASH_DEV_AUTH_BYPASS")
	}
	if c.OIDCPartiallyConfigured() && !c.OIDCConfigured() {
		return fmt.Errorf("production OIDC auth requires LIBREDASH_OIDC_ISSUER_URL, LIBREDASH_OIDC_CLIENT_ID, LIBREDASH_OIDC_CLIENT_SECRET, and LIBREDASH_OIDC_CALLBACK_URL")
	}
	if c.AzurePartiallyConfigured() && !c.AzureConfigured() {
		return fmt.Errorf("production Azure auth requires LIBREDASH_AZURE_CLIENT_ID, LIBREDASH_AZURE_CLIENT_SECRET, and LIBREDASH_AZURE_CALLBACK_URL")
	}
	if !c.OIDCConfigured() && !c.AzureConfigured() && !c.LocalAuth && !c.APITokenOnlyAuth {
		return fmt.Errorf("production serve requires OIDC auth env vars, Azure auth env vars, LIBREDASH_LOCAL_AUTH, or LIBREDASH_API_TOKEN_ONLY_AUTH")
	}
	if len(c.CSRFKey) < 32 {
		return fmt.Errorf("production serve requires LIBREDASH_CSRF_KEY with at least 32 characters")
	}
	if strings.TrimSpace(c.MetricsBearerToken) == "" {
		return fmt.Errorf("production serve requires LIBREDASH_METRICS_BEARER_TOKEN")
	}
	allowedHosts, err := c.ProductionAllowedHosts()
	if err != nil {
		return err
	}
	if len(allowedHosts) == 0 {
		return fmt.Errorf("production serve requires LIBREDASH_ALLOWED_HOSTS or an OIDC/Azure callback URL host")
	}
	cookieSecure, err := c.CookieSecure()
	if err != nil {
		return err
	}
	if !cookieSecure && !c.APITokenOnlyAuth && (c.OIDCConfigured() || c.AzureConfigured() || c.LocalAuth) {
		return fmt.Errorf("production browser auth requires LIBREDASH_COOKIE_SECURE=true")
	}
	if c.OIDCConfigured() {
		if err := requireHTTPSURL("LIBREDASH_OIDC_ISSUER_URL", c.OIDCIssuerURL); err != nil {
			return err
		}
		if err := requireHTTPSURL("LIBREDASH_OIDC_CALLBACK_URL", c.OIDCCallbackURL); err != nil {
			return err
		}
	}
	if c.AzureConfigured() {
		if err := requireHTTPSURL("LIBREDASH_AZURE_CALLBACK_URL", c.AzureCallbackURL); err != nil {
			return err
		}
	}
	if token := strings.TrimSpace(c.SCIMBearerToken); token != "" && len(token) < 32 {
		return fmt.Errorf("production SCIM provisioning requires LIBREDASH_SCIM_BEARER_TOKEN with at least 32 characters")
	}
	if token := strings.TrimSpace(c.MetricsBearerToken); token != "" && len(token) < 32 {
		return fmt.Errorf("production metrics scraping requires LIBREDASH_METRICS_BEARER_TOKEN with at least 32 characters")
	}
	return nil
}

func requireHTTPSURL(name, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("production serve requires %s to be an https URL", name)
	}
	return nil
}

func parseAllowedHosts(raw string) ([]string, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		host, err := normalizeAllowedHost(field)
		if err != nil {
			return nil, err
		}
		if host != "" {
			out = append(out, host)
		}
	}
	return out, nil
}

func callbackAllowedHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid callback URL host %q: %w", raw, err)
	}
	return normalizeAllowedHost(parsed.Host)
}

func normalizeAllowedHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", nil
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/\\") {
		return "", fmt.Errorf("LIBREDASH_ALLOWED_HOSTS entries must be hostnames, not URLs: %q", raw)
	}
	if host == "*" {
		return "", fmt.Errorf("LIBREDASH_ALLOWED_HOSTS must not allow every host in production")
	}
	if strings.HasPrefix(host, "[") {
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	} else if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if strings.HasPrefix(host, "*.") {
		suffix := strings.TrimPrefix(host, "*.")
		if suffix == "" || strings.Contains(suffix, "*") {
			return "", fmt.Errorf("invalid LIBREDASH_ALLOWED_HOSTS wildcard entry: %q", raw)
		}
		return "*." + suffix, nil
	}
	if strings.Contains(host, "*") || strings.ContainsAny(host, " \r\n\t") {
		return "", fmt.Errorf("invalid LIBREDASH_ALLOWED_HOSTS entry: %q", raw)
	}
	return host, nil
}

func (c Config) RequestLoggingEnabled() bool {
	return c.Production
}

func (c Config) RateLimitingEnabled() bool {
	return c.Production
}

func (c Config) RateLimitingUsesRealIP() bool {
	return c.Production && c.TrustProxyHeaders
}

func (c Config) HSTSEnabled(cookieSecure bool) bool {
	return c.Production && cookieSecure
}
