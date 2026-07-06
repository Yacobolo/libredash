package oidc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu      sync.Mutex
	configs map[string]Config
	clients map[string]*Client
}

func NewRegistry(configs []Config) (*Registry, error) {
	out := &Registry{
		configs: map[string]Config{},
		clients: map[string]*Client{},
	}
	for _, cfg := range configs {
		cfg.ID = strings.TrimSpace(cfg.ID)
		if cfg.ID == "" {
			return nil, errors.New("oidc provider ID is required")
		}
		if _, ok := out.configs[cfg.ID]; ok {
			return nil, fmt.Errorf("duplicate oidc provider %q", cfg.ID)
		}
		out.configs[cfg.ID] = cfg
	}
	return out, nil
}

func (r *Registry) Configured() bool {
	return r != nil && len(r.configs) > 0
}

func (r *Registry) Config(id string) (Config, bool) {
	if r == nil {
		return Config{}, false
	}
	id = defaultProviderID(id)
	cfg, ok := r.configs[id]
	return cfg, ok
}

func (r *Registry) Client(ctx context.Context, id string) (*Client, Config, error) {
	if r == nil {
		return nil, Config{}, errors.New("oidc registry is not configured")
	}
	id = defaultProviderID(id)
	r.mu.Lock()
	if client := r.clients[id]; client != nil {
		cfg := r.configs[id]
		r.mu.Unlock()
		return client, cfg, nil
	}
	cfg, ok := r.configs[id]
	r.mu.Unlock()
	if !ok {
		return nil, Config{}, fmt.Errorf("unsupported oidc provider %q", id)
	}
	client, err := New(ctx, cfg)
	if err != nil {
		return nil, Config{}, err
	}
	r.mu.Lock()
	if existing := r.clients[id]; existing != nil {
		r.mu.Unlock()
		return existing, cfg, nil
	}
	r.clients[id] = client
	r.mu.Unlock()
	return client, cfg, nil
}

func defaultProviderID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "azureadv2"
	}
	return id
}

func ProviderID(id string) string {
	return defaultProviderID(id)
}
