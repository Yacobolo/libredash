package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestRuntimeViewCollectorDelegatesOnlyAtomicIdleDeletion(t *testing.T) {
	now := time.Now().UTC()
	cache := &fakeRuntimeCache{candidates: []RuntimeViewCandidate{
		{RevisionID: "sha256:" + testDigest('a'), LastUsed: now.Add(-48 * time.Hour), Token: "one"},
		{RevisionID: "sha256:" + testDigest('b'), LastUsed: now.Add(-48 * time.Hour), Token: "two"},
	}, deleted: map[string]bool{"one": true, "two": false}}
	collector, err := NewRuntimeViewCollector(cache, RuntimeViewGCConfig{GraceAge: 24 * time.Hour, Limit: 10, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := collector.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidates != 2 || result.Deleted != 1 || len(cache.attempted) != 2 {
		t.Fatalf("Run() = %#v, attempted = %#v", result, cache.attempted)
	}
}

type fakeRuntimeCache struct {
	candidates []RuntimeViewCandidate
	deleted    map[string]bool
	attempted  []string
}

func (f *fakeRuntimeCache) ListEvictionCandidates(context.Context, time.Time, int) ([]RuntimeViewCandidate, error) {
	return append([]RuntimeViewCandidate(nil), f.candidates...), nil
}

func (f *fakeRuntimeCache) DeleteIfIdle(_ context.Context, candidate RuntimeViewCandidate) (bool, error) {
	f.attempted = append(f.attempted, candidate.Token)
	return f.deleted[candidate.Token], nil
}
