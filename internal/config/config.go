package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Yacobolo/leapview/internal/configspec"
	"github.com/Yacobolo/leapview/internal/execution"
	"github.com/caarlos0/env/v11"
)

type Profile string

const ProfileServe Profile = "serve"

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, configurationError(err)
	}
	if strings.TrimSpace(cfg.ManagedDataDir) == "" {
		cfg.ManagedDataDir = filepath.Join(cfg.HomeDir, "managed-data")
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
	return ":8080"
}

func (c Config) DBPath() string {
	return filepath.Join(c.HomeDir, "leapview.db")
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
	return filepath.Join(dir, "leapview", "cli.json")
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
	out := make([]string, 0, len(hosts)+3)
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
	for _, raw := range []string{c.PublicURL, c.OIDCCallbackURL, c.AzureCallbackURL} {
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
		return false, fmt.Errorf("LEAPVIEW_COOKIE_SECURE must be a boolean: %w", err)
	}
	return parsed, nil
}

func (c Config) Validate(profile Profile) error {
	if profile != ProfileServe {
		return fmt.Errorf("unsupported configuration profile %q", profile)
	}
	if _, err := c.AllowedHostList(); err != nil {
		return err
	}
	cookieSecure, err := c.CookieSecure()
	if err != nil {
		return err
	}
	values := c.catalogValues()
	values[configspec.EnvLEAPVIEW_COOKIE_SECURE] = cookieSecure
	return configspec.Validate(values)
}

func (c Config) ValidateProductionAuth() error {
	return c.Validate(ProfileServe)
}

func (c Config) ExecutionConfig() execution.Config {
	return execution.Config{
		MaxRunningReads:      c.ExecMaxRunningReads,
		MaxQueuedReads:       c.ExecMaxQueuedReads,
		ReadQueueWait:        c.ExecReadQueueTimeout,
		ReadExecutionTimeout: c.ExecReadTimeout,
		MaxRunningJobs:       c.ExecMaxRunningWrites,
		MaxQueuedJobs:        c.ExecMaxQueuedWrites,
	}
}

func redactSecrets(err error) error {
	message := err.Error()
	for _, setting := range configspec.Settings() {
		if !setting.Secret {
			continue
		}
		if value := os.Getenv(setting.Name); len(value) >= 8 {
			message = strings.ReplaceAll(message, value, "[REDACTED]")
		}
	}
	return fmt.Errorf("%s", message)
}

func configurationError(err error) error {
	var parseErr env.ParseError
	if errors.As(err, &parseErr) {
		for _, setting := range configspec.Settings() {
			if setting.Runtime && setting.Field == parseErr.Name {
				err = fmt.Errorf("%s must be a valid %s: %w", setting.Name, setting.Type, parseErr.Err)
				break
			}
		}
	}
	return redactSecrets(err)
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
		return "", fmt.Errorf("LEAPVIEW_ALLOWED_HOSTS entries must be hostnames, not URLs: %q", raw)
	}
	if host == "*" {
		return "", fmt.Errorf("LEAPVIEW_ALLOWED_HOSTS must not allow every host in production")
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
			return "", fmt.Errorf("invalid LEAPVIEW_ALLOWED_HOSTS wildcard entry: %q", raw)
		}
		return "*." + suffix, nil
	}
	if strings.Contains(host, "*") || strings.ContainsAny(host, " \r\n\t") {
		return "", fmt.Errorf("invalid LEAPVIEW_ALLOWED_HOSTS entry: %q", raw)
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
