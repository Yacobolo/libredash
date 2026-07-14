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
