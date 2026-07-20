package maintenance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/manageddata/runtimeview"
	"github.com/Yacobolo/leapview/internal/manageddata/storage"
)

// RuntimeViewCandidate is an opaque eviction candidate. Token must identify
// the exact cache state observed while listing candidates.
type RuntimeViewCandidate = runtimeview.EvictionCandidate

// LeasedRuntimeViewCache must atomically verify Token, prove that no query
// lease exists, prove that no materialization is in progress, and only then
// delete the revision. Returning false is the normal response to a race.
type LeasedRuntimeViewCache interface {
	ListEvictionCandidates(context.Context, time.Time, int) ([]RuntimeViewCandidate, error)
	DeleteIfIdle(context.Context, RuntimeViewCandidate) (bool, error)
}

type RuntimeViewGCConfig struct {
	GraceAge time.Duration
	Limit    int
	Now      func() time.Time
}

type RuntimeViewGCResult struct {
	Candidates int
	Deleted    int
}

type RuntimeViewCollector struct {
	cache    LeasedRuntimeViewCache
	graceAge time.Duration
	limit    int
	now      func() time.Time
}

func NewRuntimeViewCollector(cache LeasedRuntimeViewCache, config RuntimeViewGCConfig) (*RuntimeViewCollector, error) {
	if cache == nil || config.GraceAge <= 0 || config.Limit <= 0 || config.Limit > 10_000 {
		return nil, ErrInvalidMaintenance
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &RuntimeViewCollector{cache: cache, graceAge: config.GraceAge, limit: config.Limit, now: now}, nil
}

func (c *RuntimeViewCollector) Run(ctx context.Context) (RuntimeViewGCResult, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeViewGCResult{}, err
	}
	cutoff := c.now().UTC().Add(-c.graceAge)
	candidates, err := c.cache.ListEvictionCandidates(ctx, cutoff, c.limit)
	if err != nil {
		return RuntimeViewGCResult{}, sanitizeMaintenanceError(ctx, "list runtime cache candidates", err)
	}
	if len(candidates) > c.limit {
		return RuntimeViewGCResult{}, fmt.Errorf("%w: runtime cache exceeded candidate limit", storage.ErrIntegrity)
	}
	result := RuntimeViewGCResult{Candidates: len(candidates)}
	for _, candidate := range candidates {
		if err := validateRuntimeCandidate(candidate, cutoff); err != nil {
			return result, err
		}
		deleted, err := c.cache.DeleteIfIdle(ctx, candidate)
		if err != nil {
			return result, sanitizeMaintenanceError(ctx, "delete idle runtime cache view", err)
		}
		if deleted {
			result.Deleted++
		}
	}
	return result, nil
}

func validateRuntimeCandidate(candidate RuntimeViewCandidate, cutoff time.Time) error {
	const prefix = "sha256:"
	if candidate.Token == "" || candidate.LastUsed.IsZero() || candidate.LastUsed.After(cutoff) || !strings.HasPrefix(candidate.RevisionID, prefix) || storage.ValidateSHA256(strings.TrimPrefix(candidate.RevisionID, prefix)) != nil {
		return fmt.Errorf("%w: runtime cache returned an invalid eviction candidate", storage.ErrIntegrity)
	}
	return nil
}
