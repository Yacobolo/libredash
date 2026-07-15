// Package maintenance provides conservative managed-data capacity and garbage
// collection services. It does not own scheduling or persistence wiring.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/manageddata/storage"
)

var (
	ErrInvalidMaintenance  = errors.New("invalid managed-data maintenance configuration")
	ErrReachabilityChanged = errors.New("managed-data reachability changed")
)

// ReachabilitySnapshot must contain every digest that is reachable from
// durable metadata at Generation.
type ReachabilitySnapshot struct {
	Generation uint64
	SHA256s    []string
}

// ReachabilitySource supplies complete reachability snapshots. The callback
// passed to WithStableSnapshot must execute while the implementation prevents
// the generation from changing. Returning ErrReachabilityChanged is expected
// when expectedGeneration cannot be held stable.
type ReachabilitySource interface {
	Snapshot(context.Context) (ReachabilitySnapshot, error)
	WithStableSnapshot(context.Context, uint64, func(ReachabilitySnapshot) error) error
}

type BlobGCConfig struct {
	GraceAge  time.Duration
	BatchSize int
	Now       func() time.Time
}

type BlobGCResult struct {
	Candidates     int
	Deleted        int
	ReclaimedBytes int64
	Deferred       bool
}

type BlobCollector struct {
	inventory    storage.BlobInventory
	reachability ReachabilitySource
	graceAge     time.Duration
	batchSize    int
	now          func() time.Time
}

func NewBlobCollector(inventory storage.BlobInventory, reachability ReachabilitySource, config BlobGCConfig) (*BlobCollector, error) {
	if inventory == nil || reachability == nil || config.GraceAge <= 0 || config.BatchSize < 0 || config.BatchSize > 1000 {
		return nil, ErrInvalidMaintenance
	}
	batchSize := config.BatchSize
	if batchSize == 0 {
		batchSize = 100
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &BlobCollector{inventory: inventory, reachability: reachability, graceAge: config.GraceAge, batchSize: batchSize, now: now}, nil
}

func (c *BlobCollector) Run(ctx context.Context) (BlobGCResult, error) {
	if err := ctx.Err(); err != nil {
		return BlobGCResult{}, err
	}
	initial, err := c.reachability.Snapshot(ctx)
	if err != nil {
		return BlobGCResult{}, sanitizeMaintenanceError(ctx, "snapshot reachability", err)
	}
	reachable, err := digestSet(initial.SHA256s)
	if err != nil {
		return BlobGCResult{}, err
	}
	cutoff := c.now().UTC().Add(-c.graceAge)
	candidates := make(map[string]storage.BlobMetadata)
	err = c.inventory.WalkBlobs(ctx, func(blob storage.BlobMetadata) error {
		if err := validateBlobMetadata(blob); err != nil {
			return err
		}
		if previous, exists := candidates[blob.SHA256]; exists {
			if previous != blob {
				return fmt.Errorf("%w: duplicate blob inventory metadata", storage.ErrIntegrity)
			}
			return nil
		}
		if _, keep := reachable[blob.SHA256]; !keep && !blob.LastModified.After(cutoff) {
			candidates[blob.SHA256] = blob
		}
		return nil
	})
	if err != nil {
		return BlobGCResult{}, sanitizeMaintenanceError(ctx, "enumerate blobs", err)
	}
	result := BlobGCResult{Candidates: len(candidates)}
	if len(candidates) == 0 {
		return result, nil
	}
	err = c.reachability.WithStableSnapshot(ctx, initial.Generation, func(current ReachabilitySnapshot) error {
		if current.Generation != initial.Generation {
			return ErrReachabilityChanged
		}
		currentReachable, setErr := digestSet(current.SHA256s)
		if setErr != nil {
			return setErr
		}
		digests := make([]string, 0, len(candidates))
		var reclaimable int64
		for digest := range candidates {
			if _, keep := currentReachable[digest]; !keep {
				if candidates[digest].Size > math.MaxInt64-reclaimable {
					return fmt.Errorf("%w: blob inventory size overflow", storage.ErrIntegrity)
				}
				reclaimable += candidates[digest].Size
				digests = append(digests, digest)
			}
		}
		sort.Strings(digests)
		for start := 0; start < len(digests); start += c.batchSize {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := min(start+c.batchSize, len(digests))
			batch := digests[start:end]
			if err := c.inventory.DeleteBlobs(ctx, batch); err != nil {
				return err
			}
			result.Deleted += len(batch)
		}
		result.ReclaimedBytes = reclaimable
		return nil
	})
	if errors.Is(err, ErrReachabilityChanged) {
		result.Deferred = true
		return result, nil
	}
	if err != nil {
		return result, sanitizeMaintenanceError(ctx, "delete unreachable blobs", err)
	}
	return result, nil
}

func digestSet(digests []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(digests))
	for _, digest := range digests {
		if err := storage.ValidateSHA256(digest); err != nil {
			return nil, fmt.Errorf("%w: reachability contains an invalid digest", storage.ErrIntegrity)
		}
		result[digest] = struct{}{}
	}
	return result, nil
}

func validateBlobMetadata(blob storage.BlobMetadata) error {
	if storage.ValidateSHA256(blob.SHA256) != nil || blob.Size < 0 || blob.LastModified.IsZero() {
		return fmt.Errorf("%w: backend returned invalid blob metadata", storage.ErrIntegrity)
	}
	return nil
}

func sanitizeMaintenanceError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	for _, sentinel := range []error{storage.ErrInvalid, storage.ErrIntegrity, storage.ErrNotFound, storage.ErrBackend, ErrReachabilityChanged} {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("%w: %s", sentinel, operation)
		}
	}
	return fmt.Errorf("%w: %s", storage.ErrBackend, strings.TrimSpace(operation))
}
