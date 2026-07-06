package oidc

import "testing"

func TestRegistryLoadsGenericProvider(t *testing.T) {
	registry, err := NewRegistry([]Config{{
		ID:           "okta",
		IssuerURL:    "https://example.okta.com/oauth2/default",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://app.example/auth/okta/callback",
		Scopes:       []string{"groups"},
	}})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	if !registry.Configured() {
		t.Fatal("registry configured = false, want true")
	}
	cfg, ok := registry.Config("okta")
	if !ok {
		t.Fatal("okta provider missing")
	}
	if cfg.IssuerURL != "https://example.okta.com/oauth2/default" || cfg.ClientID != "client-id" || cfg.RedirectURL != "https://app.example/auth/okta/callback" {
		t.Fatalf("config = %#v", cfg)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "groups" {
		t.Fatalf("scopes = %#v, want groups", cfg.Scopes)
	}
}

func TestRegistryLoadsAzurePresetAlias(t *testing.T) {
	registry, err := NewRegistry([]Config{
		AzureProviderConfig("client-id", "client-secret", "https://app.example/auth/azureadv2/callback", "tenant-id"),
	})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	cfg, ok := registry.Config("")
	if !ok {
		t.Fatal("default azureadv2 provider missing")
	}
	if cfg.ID != "azureadv2" || cfg.IssuerURL != "https://login.microsoftonline.com/tenant-id/v2.0" {
		t.Fatalf("azure config = %#v", cfg)
	}
}

func TestRegistryRejectsDuplicateProviders(t *testing.T) {
	_, err := NewRegistry([]Config{
		{ID: "oidc", IssuerURL: "https://issuer-1.example"},
		{ID: "oidc", IssuerURL: "https://issuer-2.example"},
	})
	if err == nil {
		t.Fatal("duplicate provider error = nil")
	}
}
