package maintenance

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

func TestBlobCollectorDeletesOnlyStableUnreachableBlobs(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	reachable := testDigest('a')
	oldUnreachable := testDigest('b')
	newUnreachable := testDigest('c')
	inventory := &fakeInventory{blobs: []storage.BlobMetadata{
		{SHA256: newUnreachable, Size: 3, LastModified: now.Add(-time.Hour)},
		{SHA256: oldUnreachable, Size: 2, LastModified: now.Add(-48 * time.Hour)},
		{SHA256: reachable, Size: 1, LastModified: now.Add(-48 * time.Hour)},
	}}
	reachability := &fakeReachability{snapshot: ReachabilitySnapshot{Generation: 7, SHA256s: []string{reachable}}}
	collector, err := NewBlobCollector(inventory, reachability, BlobGCConfig{
		GraceAge:  24 * time.Hour,
		BatchSize: 1,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := collector.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Deferred || result.Candidates != 1 || result.Deleted != 1 || result.ReclaimedBytes != 2 {
		t.Fatalf("Run() = %#v", result)
	}
	if got := inventory.deleted; len(got) != 1 || len(got[0]) != 1 || got[0][0] != oldUnreachable {
		t.Fatalf("deleted batches = %#v", got)
	}
	if reachability.stableCalls != 1 {
		t.Fatalf("stable reachability calls = %d", reachability.stableCalls)
	}
}

func TestBlobCollectorDefersWhenReachabilityGenerationChanges(t *testing.T) {
	now := time.Now().UTC()
	digest := testDigest('d')
	inventory := &fakeInventory{blobs: []storage.BlobMetadata{{SHA256: digest, Size: 4, LastModified: now.Add(-48 * time.Hour)}}}
	reachability := &fakeReachability{
		snapshot:       ReachabilitySnapshot{Generation: 1},
		stableSnapshot: ReachabilitySnapshot{Generation: 2},
	}
	collector, err := NewBlobCollector(inventory, reachability, BlobGCConfig{GraceAge: time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := collector.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deferred || result.Deleted != 0 || len(inventory.deleted) != 0 {
		t.Fatalf("Run() = %#v, deleted = %#v", result, inventory.deleted)
	}
}

func TestBlobCollectorRechecksReachableSetUnderStableLease(t *testing.T) {
	now := time.Now().UTC()
	digest := testDigest('e')
	inventory := &fakeInventory{blobs: []storage.BlobMetadata{{SHA256: digest, Size: 5, LastModified: now.Add(-48 * time.Hour)}}}
	reachability := &fakeReachability{
		snapshot:       ReachabilitySnapshot{Generation: 3},
		stableSnapshot: ReachabilitySnapshot{Generation: 3, SHA256s: []string{digest}},
	}
	collector, err := NewBlobCollector(inventory, reachability, BlobGCConfig{GraceAge: time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := collector.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Candidates != 1 || result.Deleted != 0 || len(inventory.deleted) != 0 {
		t.Fatalf("Run() = %#v, deleted = %#v", result, inventory.deleted)
	}
}

func TestBlobCollectorBoundsBatchesAndHonorsCancellation(t *testing.T) {
	now := time.Now().UTC()
	inventory := &fakeInventory{}
	for index := byte(0); index < 5; index++ {
		inventory.blobs = append(inventory.blobs, storage.BlobMetadata{SHA256: testDigest('0' + index), Size: 1, LastModified: now.Add(-48 * time.Hour)})
	}
	reachability := &fakeReachability{snapshot: ReachabilitySnapshot{Generation: 1}}
	collector, err := NewBlobCollector(inventory, reachability, BlobGCConfig{GraceAge: time.Hour, BatchSize: 2, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := collector.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 5 || len(inventory.deleted) != 3 {
		t.Fatalf("Run() = %#v, batches = %#v", result, inventory.deleted)
	}
	for _, batch := range inventory.deleted {
		if len(batch) > 2 {
			t.Fatalf("oversized batch = %d", len(batch))
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := collector.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Run() error = %v", err)
	}
}

func TestBlobCollectorRejectsInvalidConfigurationAndInventory(t *testing.T) {
	reachability := &fakeReachability{snapshot: ReachabilitySnapshot{Generation: 1}}
	if _, err := NewBlobCollector(nil, reachability, BlobGCConfig{GraceAge: time.Hour}); !errors.Is(err, ErrInvalidMaintenance) {
		t.Fatalf("nil inventory error = %v", err)
	}
	if _, err := NewBlobCollector(&fakeInventory{}, nil, BlobGCConfig{GraceAge: time.Hour}); !errors.Is(err, ErrInvalidMaintenance) {
		t.Fatalf("nil reachability error = %v", err)
	}
	collector, err := NewBlobCollector(&fakeInventory{blobs: []storage.BlobMetadata{{SHA256: "unsafe", LastModified: time.Now().Add(-time.Hour)}}}, reachability, BlobGCConfig{GraceAge: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Run(t.Context()); !errors.Is(err, storage.ErrIntegrity) {
		t.Fatalf("invalid inventory error = %v", err)
	}
}

func TestBlobCollectorSanitizesRepositoryErrors(t *testing.T) {
	collector, err := NewBlobCollector(&fakeInventory{}, errorReachability{}, BlobGCConfig{GraceAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	_, err = collector.Run(t.Context())
	if !errors.Is(err, storage.ErrBackend) || strings.Contains(err.Error(), "database-password") {
		t.Fatalf("sanitized error = %v", err)
	}
}

type fakeInventory struct {
	mu      sync.Mutex
	blobs   []storage.BlobMetadata
	deleted [][]string
}

func (f *fakeInventory) WalkBlobs(ctx context.Context, visit func(storage.BlobMetadata) error) error {
	for _, blob := range f.blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(blob); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeInventory) DeleteBlobs(ctx context.Context, digests []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, append([]string(nil), digests...))
	return nil
}

type fakeReachability struct {
	snapshot       ReachabilitySnapshot
	stableSnapshot ReachabilitySnapshot
	stableCalls    int
}

type errorReachability struct{}

func (errorReachability) Snapshot(context.Context) (ReachabilitySnapshot, error) {
	return ReachabilitySnapshot{}, errors.New("database-password")
}

func (errorReachability) WithStableSnapshot(context.Context, uint64, func(ReachabilitySnapshot) error) error {
	panic("unexpected stable snapshot")
}

func (f *fakeReachability) Snapshot(context.Context) (ReachabilitySnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeReachability) WithStableSnapshot(_ context.Context, expected uint64, use func(ReachabilitySnapshot) error) error {
	f.stableCalls++
	snapshot := f.stableSnapshot
	if snapshot.Generation == 0 {
		snapshot = f.snapshot
	}
	if snapshot.Generation != expected {
		return ErrReachabilityChanged
	}
	return use(snapshot)
}

func testDigest(char byte) string {
	value := make([]byte, 64)
	for index := range value {
		value[index] = char
	}
	return string(value)
}
