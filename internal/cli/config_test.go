package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfigValidateUsesServeProfile(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "")
	cmd := configCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config validate: %v", err)
	}
	if got := output.String(); got != "configuration valid\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestConfigValidateDoesNotExposeSecretValues(t *testing.T) {
	secret := "short-secret-value"
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	t.Setenv("LIBREDASH_LOCAL_AUTH", "1")
	t.Setenv("LIBREDASH_ALLOWED_HOSTS", "libredash.example.com")
	t.Setenv("LIBREDASH_COOKIE_SECURE", "true")
	t.Setenv("LIBREDASH_CSRF_KEY", secret)
	t.Setenv("LIBREDASH_METRICS_BEARER_TOKEN", "0123456789abcdef0123456789abcdef")
	cmd := configCommand()
	cmd.SetArgs([]string{"validate"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("config validate accepted a short CSRF secret")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation exposed secret value: %v", err)
	}
}

func TestConfigValidateProductionFlagAppliesProductionRules(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "")
	cmd := configCommand()
	cmd.SetArgs([]string{"validate", "--production"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("config validate --production accepted missing production authentication")
	}
}
