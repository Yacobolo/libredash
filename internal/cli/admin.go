package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	analyticsducklake "github.com/Yacobolo/libredash/internal/analytics/ducklake"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentsqlite "github.com/Yacobolo/libredash/internal/deployment/sqlite"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/spf13/cobra"
)

func adminCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "admin", Short: "Administrative utilities"}
	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an owner principal and API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.MustLoad()
			store, err := platform.Open(ctx, cfg.DBPath())
			if err != nil {
				return err
			}
			defer store.Close()
			email := cfg.BootstrapEmail
			if email == "" {
				email = "admin@localhost"
			}
			accessRepo := accesssqlite.NewRepository(store.SQLDB())
			principal, err := accessRepo.SetPlatformRole(ctx, access.PlatformRoleInput{Email: email, DisplayName: email, Role: access.RoleAdmin})
			if err != nil {
				return err
			}
			token, err := accessRepo.CreateAPIToken(ctx, principal.ID, "bootstrap")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}
	storage := &cobra.Command{Use: "storage", Short: "Maintain analytical storage"}
	cleanup := &cobra.Command{
		Use:   "cleanup",
		Short: "Reconcile deployment snapshots and clean DuckLake storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminStorageCleanup(ctx, opts, cmd.OutOrStdout())
		},
	}
	cleanup.Flags().StringVar(&opts.environment, "environment", string(deployment.DefaultEnvironment), "deployment environment")
	cleanup.Flags().BoolVar(&opts.apply, "apply", false, "perform destructive cleanup instead of dry-run")
	storage.AddCommand(cleanup)
	parent.AddCommand(bootstrap, storage)
	return parent
}

func runAdminStorageCleanup(ctx context.Context, opts *rootOptions, out io.Writer) error {
	cfg := config.MustLoad()
	environment := deployment.NormalizeEnvironment(deployment.Environment(opts.environment))
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	repo := deploymentsqlite.NewRepository(store.SQLDB())
	referenced, err := repo.ReferencedDuckLakeSnapshots(ctx, environment)
	if err != nil {
		return err
	}
	root := filepath.Join(cfg.DuckDBDirPath(), string(environment))
	dataPath := filepath.Join(root, "data")
	env, err := analyticsducklake.Open(ctx, analyticsducklake.Config{RootDir: root, CatalogPath: cfg.DBPath(), DataPath: dataPath})
	if err != nil {
		return err
	}
	defer env.Close()
	snapshots, err := env.Snapshots(ctx)
	if err != nil {
		return err
	}
	snapshotSet := map[int64]struct{}{}
	for _, snapshot := range snapshots {
		snapshotSet[snapshot.ID] = struct{}{}
	}
	var missing []int64
	protected := map[int64]struct{}{}
	for _, snapshotID := range referenced {
		protected[snapshotID] = struct{}{}
		if _, ok := snapshotSet[snapshotID]; !ok {
			missing = append(missing, snapshotID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("deployment references missing DuckLake snapshots: %s", formatSnapshotIDs(missing))
	}
	candidates, err := env.RetentionCandidates(ctx, protected)
	if err != nil {
		return err
	}
	dryRun := !opts.apply
	fmt.Fprintf(out, "ducklake root: %s\n", root)
	fmt.Fprintf(out, "mode: %s\n", cleanupMode(dryRun))
	fmt.Fprintf(out, "protected snapshots: %s\n", formatSnapshotIDs(referenced))
	fmt.Fprintf(out, "expiration candidates: %s\n", formatSnapshotIDs(candidates))
	if err := env.ExpireSnapshots(ctx, candidates, dryRun); err != nil {
		return fmt.Errorf("expire snapshots: %w", err)
	}
	if err := env.CleanupOldFiles(ctx, dryRun); err != nil {
		return fmt.Errorf("cleanup old files: %w", err)
	}
	if err := env.DeleteOrphanedFiles(ctx, dryRun); err != nil {
		return fmt.Errorf("delete orphaned files: %w", err)
	}
	return nil
}

func cleanupMode(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "apply"
}

func formatSnapshotIDs(ids []int64) string {
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
