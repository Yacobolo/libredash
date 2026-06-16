package config

import (
	"os"
	"path/filepath"

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
