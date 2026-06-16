package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HomeDir          string `env:"LIBREDASH_HOME" envDefault:".libredash"`
	Addr             string `env:"LIBREDASH_ADDR"`
	AddrFallback     string `env:"ADDR"`
	Port             string `env:"PORT"`
	DataDir          string `env:"LIBREDASH_DATA_DIR" envDefault:".data/olist"`
	CatalogPath      string `env:"LIBREDASH_CATALOG_PATH"`
	DuckDBPath       string `env:"LIBREDASH_DUCKDB_PATH"`
	DuckDBDir        string `env:"LIBREDASH_DUCKDB_DIR"`
	Production       bool   `env:"LIBREDASH_PRODUCTION"`
	DevAuthBypass    bool   `env:"LIBREDASH_DEV_AUTH_BYPASS"`
	APITokenOnlyAuth bool   `env:"LIBREDASH_API_TOKEN_ONLY_AUTH"`
	BootstrapEmail   string `env:"LIBREDASH_BOOTSTRAP_ADMIN_EMAIL"`
	AzureClientID    string `env:"LIBREDASH_AZURE_CLIENT_ID"`
	AzureSecret      string `env:"LIBREDASH_AZURE_CLIENT_SECRET"`
	AzureCallbackURL string `env:"LIBREDASH_AZURE_CALLBACK_URL"`
	AzureTenant      string `env:"LIBREDASH_AZURE_TENANT"`
	CSRFKey          string `env:"LIBREDASH_CSRF_KEY"`
	CookieSecureRaw  string `env:"LIBREDASH_COOKIE_SECURE"`
	Target           string `env:"LIBREDASH_TARGET"`
	APIToken         string `env:"LIBREDASH_API_TOKEN"`
	CLIConfig        string `env:"LIBREDASH_CLI_CONFIG"`
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
	if !c.AzureConfigured() && !c.DevAuthBypass && !c.APITokenOnlyAuth {
		return fmt.Errorf("production serve requires Azure auth env vars, LIBREDASH_DEV_AUTH_BYPASS, or LIBREDASH_API_TOKEN_ONLY_AUTH")
	}
	if !c.DevAuthBypass && len(c.CSRFKey) < 32 {
		return fmt.Errorf("production serve requires LIBREDASH_CSRF_KEY with at least 32 characters")
	}
	return nil
}

func (c Config) RequestLoggingEnabled() bool {
	return c.Production
}

func (c Config) RateLimitingEnabled() bool {
	return c.Production
}

func (c Config) HSTSEnabled(cookieSecure bool) bool {
	return c.Production && cookieSecure
}
