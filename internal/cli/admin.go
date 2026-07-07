package cli

import (
	"context"
	"fmt"
	"io"
	"net/mail"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/platform"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/spf13/cobra"
)

func adminCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "admin", Short: "Administrative utilities"}
	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an owner principal and API token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminBootstrap(ctx, cmd.OutOrStdout())
		},
	}
	storage := &cobra.Command{Use: "storage", Short: "Maintain analytical storage"}
	cleanup := &cobra.Command{
		Use:   "cleanup",
		Short: "Reconcile serving-state snapshots and clean DuckLake storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminStorageCleanup(ctx, opts, cmd.OutOrStdout())
		},
	}
	cleanup.Flags().BoolVar(&opts.apply, "apply", false, "perform destructive cleanup instead of dry-run")
	storage.AddCommand(cleanup)
	maintenance := &cobra.Command{
		Use:   "maintenance",
		Short: "Prune bounded operational history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminMaintenance(ctx, opts, cmd.OutOrStdout())
		},
	}
	maintenance.Flags().BoolVar(&opts.apply, "apply", false, "delete rows instead of dry-run")
	maintenance.Flags().IntVar(&opts.auditDays, "audit-days", defaultAuditRetentionDays, "audit event retention in days; 0 disables audit pruning")
	maintenance.Flags().IntVar(&opts.queryDays, "query-days", defaultQueryRetentionDays, "query event retention in days; 0 disables query pruning")
	maintenance.Flags().IntVar(&opts.archivedAgentDays, "archived-agent-days", defaultArchivedAgentRetentionDays, "archived agent conversation retention in days; 0 disables archived conversation pruning")
	maintenance.Flags().IntVar(&opts.authStateDays, "auth-state-days", defaultAuthStateRetentionDays, "expired or revoked auth state retention in days; 0 disables auth-state pruning")
	backup := &cobra.Command{
		Use:   "backup",
		Short: "Create a consistent LibreDash instance backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminBackup(ctx, opts, cmd.OutOrStdout())
		},
	}
	backup.Flags().StringVar(&opts.backupOut, "out", "", "backup archive output path")
	backup.Flags().BoolVar(&opts.databaseOnly, "database-only", false, "backup only the platform SQLite database")
	restore := &cobra.Command{
		Use:   "restore",
		Short: "Restore LibreDash from a validated instance backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminRestore(ctx, opts, cmd.OutOrStdout())
		},
	}
	restore.Flags().StringVar(&opts.restoreFrom, "from", "", "backup archive path to restore")
	restore.Flags().StringVar(&opts.restoreBefore, "current-out", "", "path for a backup of the current instance before replacement")
	restore.Flags().BoolVar(&opts.confirmRestore, "confirm", false, "confirm replacement of the configured LibreDash instance")
	restore.Flags().BoolVar(&opts.databaseOnly, "database-only", false, "restore only the platform SQLite database")
	parent.AddCommand(bootstrap, storage, maintenance, backup, restore)
	return parent
}

const (
	defaultAuditRetentionDays         = 365
	defaultQueryRetentionDays         = 90
	defaultArchivedAgentRetentionDays = 180
	defaultAuthStateRetentionDays     = 30
)

func runAdminBootstrap(ctx context.Context, out io.Writer) error {
	cfg := config.MustLoad()
	email, err := bootstrapAdminEmail(cfg)
	if err != nil {
		return err
	}
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	accessRepo := accesssqlite.NewRepository(store.SQLDB())
	principal, err := accessRepo.SetPlatformRole(ctx, access.PlatformRoleInput{Email: email, DisplayName: email, Role: access.RolePlatformAdmin})
	if err != nil {
		return err
	}
	token, created, err := accessRepo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
		PrincipalID: principal.ID,
		Name:        "bootstrap",
	})
	if err != nil {
		return err
	}
	if err := revokePreviousBootstrapTokens(ctx, accessRepo, principal.ID, created.ID); err != nil {
		return err
	}
	fmt.Fprintln(out, token)
	return nil
}

func revokePreviousBootstrapTokens(ctx context.Context, repo *accesssqlite.Repository, principalID, keepID string) error {
	tokens, err := repo.ListAPITokens(ctx, principalID)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.ID == keepID || token.Name != "bootstrap" || token.RevokedAt != "" {
			continue
		}
		if err := repo.RevokeAPIToken(ctx, token.ID); err != nil {
			return err
		}
	}
	return nil
}

func bootstrapAdminEmail(cfg config.Config) (string, error) {
	email := strings.TrimSpace(cfg.BootstrapEmail)
	if email == "" {
		if cfg.Production {
			return "", fmt.Errorf("production admin bootstrap requires LIBREDASH_BOOTSTRAP_ADMIN_EMAIL")
		}
		email = "admin@localhost"
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address == "" {
		return "", fmt.Errorf("admin bootstrap requires a valid LIBREDASH_BOOTSTRAP_ADMIN_EMAIL")
	}
	return parsed.Address, nil
}

func runAdminBackup(ctx context.Context, opts *rootOptions, out io.Writer) error {
	if opts.backupOut == "" {
		return fmt.Errorf("admin backup requires --out")
	}
	cfg := config.MustLoad()
	if opts.databaseOnly {
		store, err := platform.Open(ctx, cfg.DBPath())
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Backup(ctx, opts.backupOut); err != nil {
			return err
		}
		fmt.Fprintf(out, "database backup written: %s\n", opts.backupOut)
		return nil
	}
	if err := validateFullInstanceArchiveLayout(cfg); err != nil {
		return err
	}
	if err := platform.BackupInstance(ctx, platform.InstanceBackupOptions{
		HomeDir: cfg.HomeDir,
		DBPath:  cfg.DBPath(),
		OutPath: opts.backupOut,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "instance backup written: %s\n", opts.backupOut)
	return nil
}

func runAdminRestore(ctx context.Context, opts *rootOptions, out io.Writer) error {
	if opts.restoreFrom == "" {
		return fmt.Errorf("admin restore requires --from")
	}
	if !opts.confirmRestore {
		return fmt.Errorf("admin restore requires --confirm")
	}
	cfg := config.MustLoad()
	if opts.databaseOnly {
		if err := platform.Restore(ctx, cfg.DBPath(), opts.restoreFrom, opts.restoreBefore); err != nil {
			return err
		}
		fmt.Fprintf(out, "database restored from: %s\n", opts.restoreFrom)
		if opts.restoreBefore != "" {
			fmt.Fprintf(out, "previous database backup: %s\n", opts.restoreBefore)
		}
		return nil
	}
	if err := validateFullInstanceArchiveLayout(cfg); err != nil {
		return err
	}
	if err := platform.RestoreInstance(ctx, platform.InstanceRestoreOptions{
		TargetHomeDir:    cfg.HomeDir,
		BackupPath:       opts.restoreFrom,
		CurrentBackupOut: opts.restoreBefore,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "instance restored from: %s\n", opts.restoreFrom)
	if opts.restoreBefore != "" {
		fmt.Fprintf(out, "previous instance backup: %s\n", opts.restoreBefore)
	}
	return nil
}

func validateFullInstanceArchiveLayout(cfg config.Config) error {
	homeAbs, err := filepath.Abs(cfg.HomeDir)
	if err != nil {
		return err
	}
	catalogAbs, err := filepath.Abs(cfg.DuckLakeCatalogPath())
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(homeAbs, catalogAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("full instance backup/restore requires DuckLake catalog path inside LIBREDASH_HOME; got %s outside %s", cfg.DuckLakeCatalogPath(), cfg.HomeDir)
	}
	return nil
}

func runAdminStorageCleanup(ctx context.Context, opts *rootOptions, out io.Writer) error {
	cfg := config.MustLoad()
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	repo := servingstatesqlite.NewRepository(store.SQLDB())
	_, err = storagemaintenance.Run(ctx, repo, storagemaintenance.Options{
		RootDir:     cfg.HomeDir,
		CatalogPath: cfg.DuckLakeCatalogPath(),
		DataPath:    cfg.DuckLakeDataDir(),
		DryRun:      !opts.apply,
		Out:         out,
	})
	if err != nil {
		return fmt.Errorf("storage cleanup: %w", err)
	}
	return nil
}

func runAdminMaintenance(ctx context.Context, opts *rootOptions, out io.Writer) error {
	if opts.auditDays < 0 || opts.queryDays < 0 || opts.archivedAgentDays < 0 || opts.authStateDays < 0 {
		return fmt.Errorf("retention days must be zero or greater")
	}
	cfg := config.MustLoad()
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := store.PruneOperationalHistory(ctx, platform.OperationalRetentionOptions{
		AuditEventsMaxAge:             days(opts.auditDays),
		QueryEventsMaxAge:             days(opts.queryDays),
		ArchivedAgentConversationsAge: days(opts.archivedAgentDays),
		AuthStateMaxAge:               days(opts.authStateDays),
		DryRun:                        !opts.apply,
	})
	if err != nil {
		return fmt.Errorf("operational maintenance: %w", err)
	}
	mode := "dry-run"
	if opts.apply {
		mode = "apply"
	}
	fmt.Fprintf(out, "mode: %s\n", mode)
	fmt.Fprintf(out, "audit events: %d\n", result.AuditEventsDeleted)
	fmt.Fprintf(out, "query events: %d\n", result.QueryEventsDeleted)
	fmt.Fprintf(out, "archived agent conversations: %d\n", result.ArchivedAgentConversationsDeleted)
	fmt.Fprintf(out, "expired oauth states: %d\n", result.ExpiredOAuthStatesDeleted)
	fmt.Fprintf(out, "stale sessions: %d\n", result.StaleSessionsDeleted)
	fmt.Fprintf(out, "stale api tokens: %d\n", result.StaleAPITokensDeleted)
	fmt.Fprintf(out, "stale service principal secrets: %d\n", result.StaleServicePrincipalSecretsDeleted)
	return nil
}

func days(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * 24 * time.Hour
}
