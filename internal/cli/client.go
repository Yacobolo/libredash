package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/config"
)

func doJSON(ctx context.Context, method, endpoint, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, endpoint, strings.TrimSpace(string(bytes)))
	}
	if out == nil || len(bytes) == 0 {
		return nil
	}
	return json.Unmarshal(bytes, out)
}

type clientConfig struct {
	Targets map[string]clientTarget `json:"targets"`
}

type clientTarget struct {
	Token string `json:"token"`
}

func targetEnvironment(ctx context.Context, client *http.Client, target, token, asserted string) (string, error) {
	instance, err := newManagedDataCLIClient(client, target, token).instance(ctx)
	if err != nil {
		return "", fmt.Errorf("read target instance: %w", err)
	}
	environment := strings.TrimSpace(instance.Environment)
	if environment == "" {
		return "", fmt.Errorf("target instance returned an empty environment")
	}
	if asserted = strings.TrimSpace(asserted); asserted != "" && asserted != environment {
		return "", fmt.Errorf("requested environment %q does not match target instance environment %q", asserted, environment)
	}
	return environment, nil
}

func clientTargetAndToken(opts *rootOptions) (string, string, error) {
	cfg := config.MustLoad()
	target := strings.TrimRight(opts.target, "/")
	if target == "" {
		target = strings.TrimRight(cfg.Target, "/")
	}
	token := opts.token
	if token == "" {
		token = cfg.APIToken
	}
	config, _ := loadClientConfig()
	if target != "" && token == "" {
		token = config.Targets[target].Token
	}
	if target == "" {
		return "", "", fmt.Errorf("target is required")
	}
	if token == "" {
		return "", "", fmt.Errorf("API token is required; use --token, LIBREDASH_API_TOKEN, or libredash login --target %s --token <token>", target)
	}
	return target, token, nil
}

func loadClientConfig() (clientConfig, error) {
	path := clientConfigPath()
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return clientConfig{Targets: map[string]clientTarget{}}, nil
	}
	if err != nil {
		return clientConfig{}, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return clientConfig{}, err
	}
	var config clientConfig
	if err := json.Unmarshal(bytes, &config); err != nil {
		return clientConfig{}, err
	}
	if config.Targets == nil {
		config.Targets = map[string]clientTarget{}
	}
	return config, nil
}

func saveClientConfig(config clientConfig) error {
	path := clientConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o600)
}

func clientConfigPath() string {
	return config.MustLoad().ClientConfigPath()
}

func shortDigest(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func init() {
	http.DefaultClient.Timeout = 5 * time.Minute
}
