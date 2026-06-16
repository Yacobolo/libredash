package config

import "testing"

func TestValidateProductionAuthRequiresCSRFKey(t *testing.T) {
	cfg := Config{Production: true, APITokenOnlyAuth: true}
	if err := cfg.ValidateProductionAuth(); err == nil {
		t.Fatal("expected missing CSRF key to fail production auth validation")
	}
}

func TestValidateProductionAuthAllowsDevBypassWithoutCSRFKey(t *testing.T) {
	cfg := Config{Production: true, DevAuthBypass: true}
	if err := cfg.ValidateProductionAuth(); err != nil {
		t.Fatalf("validate production auth: %v", err)
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
	if cfg.HSTSEnabled(true) {
		t.Fatal("development HSTS = true, want false")
	}
}
