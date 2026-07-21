package app

import (
	"context"
	"testing"
)

func TestPublicationStreamRegistryClosesStaleGenerationAndPublicID(t *testing.T) {
	registry := newPublicationStreamRegistry()
	ctx, unregister := registry.Register(context.Background(), "publication", "stream", publicationStreamVersion{
		PublicID: "public-old", ServingStateID: "state-old",
	})
	defer unregister()

	registry.CloseStale(map[string]publicationStreamVersion{
		"publication": {PublicID: "public-new", ServingStateID: "state-new"},
	})
	select {
	case <-ctx.Done():
	default:
		t.Fatal("stale publication stream remained active")
	}
}

func TestPublicationStreamRegistryKeepsCurrentGeneration(t *testing.T) {
	registry := newPublicationStreamRegistry()
	version := publicationStreamVersion{PublicID: "public", ServingStateID: "state"}
	ctx, unregister := registry.Register(context.Background(), "publication", "stream", version)
	defer unregister()

	registry.CloseStale(map[string]publicationStreamVersion{"publication": version})
	select {
	case <-ctx.Done():
		t.Fatal("current publication stream was closed")
	default:
	}
}
