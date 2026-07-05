package cli

import (
	"context"
	"fmt"
	"io"

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
		Short: "Reconcile serving-state snapshots and clean DuckLake storage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdminStorageCleanup(ctx, opts, cmd.OutOrStdout())
		},
	}
	cleanup.Flags().BoolVar(&opts.apply, "apply", false, "perform destructive cleanup instead of dry-run")
	storage.AddCommand(cleanup)
	parent.AddCommand(bootstrap, storage)
	return parent
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
		CatalogPath: cfg.DBPath(),
		DataPath:    cfg.DuckLakeDataDir(),
		DryRun:      !opts.apply,
		Out:         out,
	})
	if err != nil {
		return fmt.Errorf("storage cleanup: %w", err)
	}
	return nil
}
