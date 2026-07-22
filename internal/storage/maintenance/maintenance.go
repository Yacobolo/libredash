package maintenance

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	analyticsducklake "github.com/Yacobolo/leapview/internal/analytics/ducklake"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

type ServingStateRepository interface {
	ReconcileRetention(ctx context.Context, environment servingstate.Environment, now time.Time) error
	ReferencedDuckLakeSnapshots(ctx context.Context, environment string) ([]int64, error)
}

type snapshotProtectionRepository interface {
	ActiveDuckLakeSnapshots(ctx context.Context, environment string) ([]int64, error)
	LeasedDuckLakeSnapshots(ctx context.Context, environment string) ([]int64, error)
}

type expiredLeaseReconciler interface {
	ReleaseExpiredQuerySnapshotLeases(ctx context.Context, environment string) error
}

type foreignSnapshotProtectionRepository interface {
	ForeignEnvironmentDuckLakeSnapshots(ctx context.Context, environment string) ([]int64, error)
}

type Options struct {
	DuckDBEnvironment            *analyticsducklake.Environment
	RootDir                      string
	CatalogPath                  string
	DataPath                     string
	AdditionalProtectedSnapshots []int64
	Environment                  servingstate.Environment
	DryRun                       bool
	Out                          io.Writer
}

type Report struct {
	ProtectedSnapshots        []int64
	ActiveProtectedSnapshots  []int64
	LeaseProtectedSnapshots   []int64
	ForeignProtectedSnapshots []int64
	Candidates                []int64
}

func Run(ctx context.Context, repo ServingStateRepository, options Options) (Report, error) {
	if repo == nil {
		return Report{}, fmt.Errorf("serving state repository is required")
	}
	environment := servingstate.Environment(strings.TrimSpace(string(options.Environment)))
	if environment == "" {
		return Report{}, fmt.Errorf("instance environment is required")
	}
	if !options.DryRun {
		if leases, ok := repo.(expiredLeaseReconciler); ok {
			if err := leases.ReleaseExpiredQuerySnapshotLeases(ctx, string(environment)); err != nil {
				return Report{}, err
			}
		}
		if err := repo.ReconcileRetention(ctx, environment, time.Now()); err != nil {
			return Report{}, err
		}
	}
	active, leased, foreign, err := protectedSnapshots(ctx, repo, string(environment))
	if err != nil {
		return Report{}, err
	}
	env := options.DuckDBEnvironment
	if env == nil {
		var err error
		env, err = analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: options.RootDir, CatalogPath: options.CatalogPath, DataPath: options.DataPath})
		if err != nil {
			return Report{}, err
		}
		defer env.Close()
	}
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		return Report{}, err
	}
	snapshotSet := map[int64]struct{}{}
	for _, snapshot := range snapshots {
		snapshotSet[snapshot.ID] = struct{}{}
	}
	protected := map[int64]struct{}{}
	for _, snapshotID := range active {
		protected[snapshotID] = struct{}{}
	}
	for _, snapshotID := range leased {
		protected[snapshotID] = struct{}{}
	}
	for _, snapshotID := range foreign {
		protected[snapshotID] = struct{}{}
	}
	for _, snapshotID := range options.AdditionalProtectedSnapshots {
		if snapshotID > 0 {
			protected[snapshotID] = struct{}{}
		}
	}
	protectedList := snapshotKeys(protected)
	var missing []int64
	for _, snapshotID := range protectedList {
		if _, ok := snapshotSet[snapshotID]; !ok {
			missing = append(missing, snapshotID)
		}
	}
	if len(missing) > 0 {
		return Report{}, fmt.Errorf("serving states reference missing DuckLake snapshots: %s", FormatSnapshotIDs(missing))
	}
	candidates, err := env.RetentionCandidates(ctx, protected)
	if err != nil {
		return Report{}, err
	}
	if options.Out != nil {
		fmt.Fprintf(options.Out, "ducklake catalog: %s\n", options.CatalogPath)
		fmt.Fprintf(options.Out, "ducklake data: %s\n", options.DataPath)
		fmt.Fprintf(options.Out, "mode: %s\n", cleanupMode(options.DryRun))
		fmt.Fprintf(options.Out, "protected active snapshots: %s\n", FormatSnapshotIDs(active))
		fmt.Fprintf(options.Out, "protected leased snapshots: %s\n", FormatSnapshotIDs(leased))
		fmt.Fprintf(options.Out, "protected foreign-environment snapshots: %s\n", FormatSnapshotIDs(foreign))
		fmt.Fprintf(options.Out, "protected snapshots: %s\n", FormatSnapshotIDs(protectedList))
		fmt.Fprintf(options.Out, "expiration candidates: %s\n", FormatSnapshotIDs(candidates))
	}
	if options.DryRun {
		return Report{
			ProtectedSnapshots:        protectedList,
			ActiveProtectedSnapshots:  active,
			LeaseProtectedSnapshots:   leased,
			ForeignProtectedSnapshots: foreign,
			Candidates:                candidates,
		}, nil
	}
	if err := env.ExpireSnapshots(ctx, candidates, options.DryRun); err != nil {
		return Report{}, fmt.Errorf("expire snapshots: %w", err)
	}
	if err := env.CleanupOldFiles(ctx, options.DryRun); err != nil {
		return Report{}, fmt.Errorf("cleanup old files: %w", err)
	}
	if err := env.DeleteOrphanedFiles(ctx, options.DryRun); err != nil {
		return Report{}, fmt.Errorf("delete orphaned files: %w", err)
	}
	return Report{
		ProtectedSnapshots:        protectedList,
		ActiveProtectedSnapshots:  active,
		LeaseProtectedSnapshots:   leased,
		ForeignProtectedSnapshots: foreign,
		Candidates:                candidates,
	}, nil
}

func protectedSnapshots(ctx context.Context, repo ServingStateRepository, environment string) ([]int64, []int64, []int64, error) {
	var foreign []int64
	if provider, ok := repo.(foreignSnapshotProtectionRepository); ok {
		var err error
		foreign, err = provider.ForeignEnvironmentDuckLakeSnapshots(ctx, environment)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if split, ok := repo.(snapshotProtectionRepository); ok {
		active, err := split.ActiveDuckLakeSnapshots(ctx, environment)
		if err != nil {
			return nil, nil, nil, err
		}
		leased, err := split.LeasedDuckLakeSnapshots(ctx, environment)
		if err != nil {
			return nil, nil, nil, err
		}
		return positiveSnapshotIDs(active), positiveSnapshotIDs(leased), positiveSnapshotIDs(foreign), nil
	}
	referenced, err := repo.ReferencedDuckLakeSnapshots(ctx, environment)
	if err != nil {
		return nil, nil, nil, err
	}
	return positiveSnapshotIDs(referenced), nil, positiveSnapshotIDs(foreign), nil
}

func FormatSnapshotIDs(ids []int64) string {
	if len(ids) == 0 {
		return "none"
	}
	ids = append([]int64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func cleanupMode(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func snapshotKeys(values map[int64]struct{}) []int64 {
	if len(values) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func positiveSnapshotIDs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
