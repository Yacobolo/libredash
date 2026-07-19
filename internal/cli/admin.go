package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	accesssqlite "github.com/Yacobolo/libredash/internal/access/sqlite"
	"github.com/Yacobolo/libredash/internal/config"
	"github.com/Yacobolo/libredash/internal/instancelock"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/securefs"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatesqlite "github.com/Yacobolo/libredash/internal/servingstate/sqlite"
	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
	"github.com/spf13/cobra"
)

func adminCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "admin", Short: "Administrative utilities"}
	initializeFormat := "json"
	acknowledgeCredentials := false
	initialize := &cobra.Command{
		Use:   "initialize",
		Short: "Initialize one instance administrator and publisher credential",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if acknowledgeCredentials {
				return acknowledgeInitialCredentials(ctx)
			}
			return runAdminInitialize(ctx, initializeFormat, cmd.OutOrStdout())
		},
	}
	initialize.Flags().StringVar(&initializeFormat, "format", "json", "output format (json)")
	initialize.Flags().BoolVar(&acknowledgeCredentials, "acknowledge-credentials", false, "remove the recoverable initialization credential bundle after it has been stored safely")
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
			return runAdminRestore(ctx, opts, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	restore.Flags().StringVar(&opts.restoreFrom, "from", "", "backup archive path to restore")
	restore.Flags().StringVar(&opts.restoreBefore, "current-out", "", "path for a backup of the current instance before replacement; - creates and discards a validated temporary checkpoint")
	restore.Flags().BoolVar(&opts.confirmRestore, "confirm", false, "confirm replacement of the configured LibreDash instance")
	restore.Flags().BoolVar(&opts.databaseOnly, "database-only", false, "restore only the platform SQLite database")
	parent.AddCommand(initialize, storage, maintenance, backup, restore)
	return parent
}

var errInstanceAlreadyInitialized = errors.New("LibreDash instance is already initialized")

const (
	instanceInitializedSetting        = "instance.initialized"
	initialCredentialRecoveryFileName = ".initial-credentials.json"
)

type initialInstanceCredentials struct {
	Email                   string `json:"email"`
	TemporaryPassword       string `json:"temporaryPassword"`
	PublisherToken          string `json:"publisherToken"`
	PublisherTokenExpiresAt string `json:"publisherTokenExpiresAt"`
}

func runAdminInitialize(ctx context.Context, format string, out io.Writer) error {
	if format != "json" {
		return fmt.Errorf("admin initialize supports only --format json")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lock, err := instancelock.Acquire(cfg.HomeDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	environment := serveEnvironment(cfg.Production, "", cfg.Environment)
	if err := store.BindInstanceEnvironment(ctx, string(environment)); err != nil {
		return err
	}
	recoveryPath := initialCredentialRecoveryPath(cfg.HomeDir)
	if _, err := store.GetSetting(ctx, instanceInitializedSetting); err == nil {
		credentials, readErr := readInitialCredentialRecovery(recoveryPath)
		if readErr == nil {
			return writeAll(out, credentials)
		}
		if os.IsNotExist(readErr) {
			return errInstanceAlreadyInitialized
		}
		return readErr
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := os.Remove(recoveryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale initialization credentials: %w", err)
	}
	email, err := initialAdminEmail(cfg)
	if err != nil {
		return err
	}
	repo := accesssqlite.NewRepository(store.SQLDB())
	var result initialInstanceCredentials
	var encodedResult []byte
	err = repo.RunAuditedMutationBatch(ctx, func(txRepo access.Repository) ([]access.AuditEventInput, error) {
		sqliteRepo, ok := txRepo.(*accesssqlite.Repository)
		if !ok {
			return nil, fmt.Errorf("initialize access transaction is unavailable")
		}
		inserted, err := sqliteRepo.InsertPlatformSettingIfMissing(ctx, instanceInitializedSetting, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return nil, err
		}
		if !inserted {
			return nil, errInstanceAlreadyInitialized
		}
		created, err := txRepo.CreateLocalUser(ctx, access.LocalUserInput{Email: email, DisplayName: email, MustChange: true})
		if err != nil {
			return nil, err
		}
		principal, err := txRepo.SetPlatformRole(ctx, access.PlatformRoleInput{PrincipalID: created.Principal.ID, Email: email, DisplayName: email, Role: access.RolePlatformAdmin})
		if err != nil {
			return nil, err
		}
		expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
		token, _, err := txRepo.CreateAPITokenWithMetadata(ctx, access.APITokenInput{
			PrincipalID: principal.ID,
			Name:        "initial-publisher",
			Privileges: []access.Privilege{
				access.PrivilegeUseWorkspace, access.PrivilegeViewItem, access.PrivilegeQueryData,
				access.PrivilegeRefreshData, access.PrivilegeDeploy, access.PrivilegeActivateDeployment,
				access.PrivilegeViewData, access.PrivilegeIngestData,
			},
			ExpiresAt: expires,
		})
		if err != nil {
			return nil, err
		}
		result = initialInstanceCredentials{Email: email, TemporaryPassword: created.Password, PublisherToken: token, PublisherTokenExpiresAt: expires.Format(time.RFC3339)}
		encodedResult, err = json.Marshal(result)
		if err != nil {
			return nil, err
		}
		encodedResult = append(encodedResult, '\n')
		if err := writeInitialCredentialRecovery(recoveryPath, encodedResult); err != nil {
			return nil, err
		}
		return []access.AuditEventInput{{PrincipalID: principal.ID, Action: "instance.initialized", TargetType: "instance", TargetID: string(environment), Privilege: access.PrivilegeManagePlatform, Status: "success"}}, nil
	})
	if err != nil {
		_ = os.Remove(recoveryPath)
		return err
	}
	return writeAll(out, encodedResult)
}

func acknowledgeInitialCredentials(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lock, err := instancelock.Acquire(cfg.HomeDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	if _, err := offlineInstanceEnvironment(ctx, store, cfg); err != nil {
		return err
	}
	if _, err := store.GetSetting(ctx, instanceInitializedSetting); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("LibreDash instance has not been initialized")
		}
		return err
	}
	if err := os.Remove(initialCredentialRecoveryPath(cfg.HomeDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("acknowledge initialization credentials: %w", err)
	}
	return nil
}

func initialCredentialRecoveryPath(homeDir string) string {
	return filepath.Join(homeDir, initialCredentialRecoveryFileName)
}

func readInitialCredentialRecovery(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("initialization credential recovery file %q must be a private regular file", path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var credentials initialInstanceCredentials
	if err := json.Unmarshal(contents, &credentials); err != nil || credentials.Email == "" || credentials.TemporaryPassword == "" || credentials.PublisherToken == "" {
		return nil, fmt.Errorf("initialization credential recovery file %q is invalid", path)
	}
	return contents, nil
}

func writeInitialCredentialRecovery(path string, contents []byte) error {
	if err := securefs.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".initial-credentials-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(securefs.PrivateFileMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return err
	}
	return nil
}

func writeAll(out io.Writer, contents []byte) error {
	written, err := out.Write(contents)
	if err == nil && written != len(contents) {
		return io.ErrShortWrite
	}
	return err
}

const (
	defaultAuditRetentionDays         = 365
	defaultQueryRetentionDays         = 90
	defaultArchivedAgentRetentionDays = 180
	defaultAuthStateRetentionDays     = 30
)

func initialAdminEmail(cfg config.Config) (string, error) {
	email := strings.TrimSpace(cfg.BootstrapEmail)
	if email == "" {
		if cfg.Production {
			return "", fmt.Errorf("production instance initialization requires LIBREDASH_BOOTSTRAP_ADMIN_EMAIL")
		}
		email = "admin@localhost"
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address == "" {
		return "", fmt.Errorf("instance initialization requires a valid LIBREDASH_BOOTSTRAP_ADMIN_EMAIL")
	}
	return parsed.Address, nil
}

func runAdminBackup(ctx context.Context, opts *rootOptions, out io.Writer) error {
	if opts.backupOut == "" {
		return fmt.Errorf("admin backup requires --out")
	}
	backupPath := opts.backupOut
	stream := backupPath == "-"
	cfg := config.MustLoad()
	if stream && opts.databaseOnly {
		var err error
		backupPath, err = unusedTemporaryPathIn(filepath.Dir(cfg.HomeDir), "libredash-backup-*.db")
		if err != nil {
			return err
		}
		defer os.Remove(backupPath)
	}
	lock, err := instancelock.Acquire(cfg.HomeDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	if opts.databaseOnly {
		store, err := platform.Open(ctx, cfg.DBPath())
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Backup(ctx, backupPath); err != nil {
			return err
		}
		if stream {
			return copyFile(out, backupPath)
		}
		fmt.Fprintf(out, "database backup written: %s\n", backupPath)
		return nil
	}
	if err := validateFullInstanceArchiveLayout(cfg); err != nil {
		return err
	}
	if stream {
		return platform.BackupInstanceToWriter(ctx, cfg.HomeDir, cfg.DBPath(), out)
	}
	if err := platform.BackupInstance(ctx, platform.InstanceBackupOptions{
		HomeDir: cfg.HomeDir,
		DBPath:  cfg.DBPath(),
		OutPath: backupPath,
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "instance backup written: %s\n", backupPath)
	return nil
}

func runAdminRestore(ctx context.Context, opts *rootOptions, in io.Reader, out io.Writer) error {
	if opts.restoreFrom == "" {
		return fmt.Errorf("admin restore requires --from")
	}
	if !opts.confirmRestore {
		return fmt.Errorf("admin restore requires --confirm")
	}
	cfg := config.MustLoad()
	restorePath := opts.restoreFrom
	restoreLabel := restorePath
	stream := restorePath == "-"
	if stream {
		restoreLabel = "stdin"
	}
	if stream && opts.databaseOnly {
		if in == nil {
			return fmt.Errorf("admin restore --from - requires standard input")
		}
		var err error
		restorePath, err = copyReaderToTemporaryFile(in, filepath.Dir(cfg.HomeDir), "libredash-restore-*.db")
		if err != nil {
			return err
		}
		defer os.Remove(restorePath)
	}
	if stream && !opts.databaseOnly && in == nil {
		return fmt.Errorf("admin restore --from - requires standard input")
	}
	restoreBefore := opts.restoreBefore
	if restoreBefore == "-" {
		var err error
		restoreBefore, err = unusedTemporaryPathIn(filepath.Dir(cfg.HomeDir), "libredash-current-backup-*.tar.gz")
		if err != nil {
			return err
		}
		defer os.Remove(restoreBefore)
	}
	lock, err := instancelock.Acquire(cfg.HomeDir)
	if err != nil {
		return err
	}
	defer lock.Release()
	expectedEnvironment, err := restoreTargetEnvironment(ctx, cfg)
	if err != nil {
		return err
	}
	if opts.databaseOnly {
		if err := platform.ValidateDatabaseInstanceEnvironment(ctx, restorePath, string(expectedEnvironment)); err != nil {
			return err
		}
		if err := platform.Restore(ctx, cfg.DBPath(), restorePath, restoreBefore); err != nil {
			return err
		}
		fmt.Fprintf(out, "database restored from: %s\n", restoreLabel)
		if restoreBefore != "" && opts.restoreBefore != "-" {
			fmt.Fprintf(out, "previous database backup: %s\n", restoreBefore)
		}
		return nil
	}
	if err := validateFullInstanceArchiveLayout(cfg); err != nil {
		return err
	}
	restoreOptions := platform.InstanceRestoreOptions{
		TargetHomeDir:        cfg.HomeDir,
		BackupPath:           restorePath,
		CurrentBackupOut:     restoreBefore,
		ExpectedEnvironment:  string(expectedEnvironment),
		PreserveRelativeFile: instancelock.FileName,
	}
	if stream {
		restoreOptions.BackupPath = ""
		err = platform.RestoreInstanceFromReader(ctx, restoreOptions, in)
	} else {
		err = platform.RestoreInstance(ctx, restoreOptions)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "instance restored from: %s\n", restoreLabel)
	if restoreBefore != "" && opts.restoreBefore != "-" {
		fmt.Fprintf(out, "previous instance backup: %s\n", restoreBefore)
	}
	return nil
}

func unusedTemporaryPathIn(directory, pattern string) (string, error) {
	if err := os.MkdirAll(directory, securefs.PrivateDirMode); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func copyFile(out io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(out, file)
	return err
}

func copyReaderToTemporaryFile(in io.Reader, directory, pattern string) (string, error) {
	if err := os.MkdirAll(directory, securefs.PrivateDirMode); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(securefs.PrivateFileMode); err != nil {
		_ = file.Close()
		return "", err
	}
	if _, err := io.Copy(file, in); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	cleanup = false
	return path, nil
}

func restoreTargetEnvironment(ctx context.Context, cfg config.Config) (servingstate.Environment, error) {
	if _, err := os.Stat(cfg.DBPath()); err == nil {
		store, err := platform.Open(ctx, cfg.DBPath())
		if err != nil {
			return "", err
		}
		environment, environmentErr := offlineInstanceEnvironment(ctx, store, cfg)
		closeErr := store.Close()
		if environmentErr != nil {
			return "", environmentErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return environment, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return serveEnvironment(cfg.Production, "", cfg.Environment), nil
}

func validateFullInstanceArchiveLayout(cfg config.Config) error {
	homeAbs, err := filepath.Abs(cfg.HomeDir)
	if err != nil {
		return err
	}
	paths := map[string]string{"DuckLake catalog": cfg.DuckLakeCatalogPath(), "DuckLake data": cfg.DuckLakeDataDir(), "artifact": cfg.ArtifactDir(), "runtime": cfg.RuntimeDir()}
	if cfg.ManagedDataBackend == "local" || cfg.ManagedDataBackend == "" {
		paths["managed-data"] = cfg.ManagedDataDir
	}
	for label, path := range paths {
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(homeAbs, pathAbs)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("full instance backup/restore requires %s path inside LIBREDASH_HOME; got %s outside %s", label, path, cfg.HomeDir)
		}
	}
	return nil
}

func runAdminStorageCleanup(ctx context.Context, opts *rootOptions, out io.Writer) error {
	cfg := config.MustLoad()
	lock, err := acquireDestructiveMaintenanceLock(cfg, opts.apply)
	if err != nil {
		return err
	}
	defer lock.Release()
	store, err := platform.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer store.Close()
	repo := servingstatesqlite.NewRepository(store.SQLDB())
	environment, err := offlineInstanceEnvironment(ctx, store, cfg)
	if err != nil {
		return err
	}
	_, err = storagemaintenance.Run(ctx, repo, storagemaintenance.Options{
		Environment: environment,
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

func offlineInstanceEnvironment(ctx context.Context, store *platform.Store, cfg config.Config) (servingstate.Environment, error) {
	bound, err := store.InstanceEnvironment(ctx)
	if err == nil {
		if requested := strings.TrimSpace(cfg.Environment); requested != "" && requested != bound {
			return "", fmt.Errorf("LibreDash instance is bound to environment %q, not %q", bound, requested)
		}
		return servingstate.Environment(bound), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read instance environment: %w", err)
	}
	environment := serveEnvironment(cfg.Production, "", cfg.Environment)
	if err := store.BindInstanceEnvironment(ctx, string(environment)); err != nil {
		return "", err
	}
	return environment, nil
}

func runAdminMaintenance(ctx context.Context, opts *rootOptions, out io.Writer) error {
	if opts.auditDays < 0 || opts.queryDays < 0 || opts.archivedAgentDays < 0 || opts.authStateDays < 0 {
		return fmt.Errorf("retention days must be zero or greater")
	}
	cfg := config.MustLoad()
	lock, err := acquireDestructiveMaintenanceLock(cfg, opts.apply)
	if err != nil {
		return err
	}
	defer lock.Release()
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

func acquireDestructiveMaintenanceLock(cfg config.Config, apply bool) (*instancelock.Lock, error) {
	if !apply {
		return nil, nil
	}
	return instancelock.Acquire(cfg.HomeDir)
}

func days(value int) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * 24 * time.Hour
}
