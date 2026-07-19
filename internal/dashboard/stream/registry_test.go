package stream

import (
	"context"
	"testing"
	"time"
)

func TestRegistryExpiresOrphanCoordinatorCreatedByCommand(t *testing.T) {
	registry := NewRegistryWithTTL(10 * time.Millisecond)
	coordinator := registry.Ensure("client:page:instance", context.Background(), func(RefreshEvent) {})
	canceled := make(chan struct{})
	if _, err := coordinator.Begin(nil, func(ctx context.Context, _ RefreshPublisher) {
		<-ctx.Done()
		close(canceled)
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("orphan coordinator was not canceled at TTL")
	}
	if _, ok := registry.Get("client:page:instance"); ok {
		t.Fatal("expired orphan remains registered")
	}
}

func TestRegistryRefreshSemanticModelTargetsOnlyMatchingStreams(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()
	for _, streamID := range []string{"sales-a", "sales-b", "support"} {
		registry.Ensure(streamID, ctx, func(RefreshEvent) {})
	}
	refreshed := map[string]int{}
	registry.Bind("sales-a", "sales", "prod", "orders", func() { refreshed["sales-a"]++ })
	registry.Bind("sales-b", "sales", "prod", "customers", func() { refreshed["sales-b"]++ })
	registry.Bind("support", "support", "prod", "orders", func() { refreshed["support"]++ })
	registry.Ensure("sales-dev", ctx, func(RefreshEvent) {})
	registry.Bind("sales-dev", "sales", "dev", "orders", func() { refreshed["sales-dev"]++ })

	streams := registry.RefreshSemanticModel("sales", "prod", "orders")
	if len(streams) != 1 || streams[0] != "sales-a" {
		t.Fatalf("refreshed streams = %#v, want sales-a", streams)
	}
	if refreshed["sales-a"] != 1 || refreshed["sales-b"] != 0 || refreshed["support"] != 0 || refreshed["sales-dev"] != 0 {
		t.Fatalf("refresh callbacks = %#v", refreshed)
	}
}
