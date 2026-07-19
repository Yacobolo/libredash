package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	addr               string
	production         bool
	workspaceID        string
	environment        string
	target             string
	token              string
	catalog            string
	conversation       string
	jsonOutput         bool
	pageID             string
	count              int
	filtersJSON        string
	bodyJSON           string
	schemaFormat       string
	schemaOut          string
	backupOut          string
	restoreFrom        string
	restoreBefore      string
	confirmRestore     bool
	databaseOnly       bool
	auditDays          int
	queryDays          int
	archivedAgentDays  int
	authStateDays      int
	limit              int
	pageToken          string
	searchTypes        []string
	autoApprove        bool
	apply              bool
	healthcheckURL     string
	healthcheckTimeout time.Duration
}

func Execute(ctx context.Context) error {
	return NewCommand(ctx).ExecuteContext(ctx)
}

// NewCommand constructs the LibreDash CLI command tree for execution and documentation.
func NewCommand(ctx context.Context) *cobra.Command {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:   "libredash",
		Short: "LibreDash BI-as-code server and deployment CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.environment = ""
			return runServe(ctx, opts)
		},
	}
	root.AddCommand(serveCommand(ctx, opts))
	root.AddCommand(deployCommand(ctx, opts))
	root.AddCommand(validateCommand(ctx, opts))
	root.AddCommand(planCommand(ctx, opts))
	root.AddCommand(schemaCommand(opts))
	root.AddCommand(configCommand())
	root.AddCommand(dataCommand(ctx, opts))
	root.AddCommand(apiCommand(ctx, opts))
	root.AddCommand(agentCommand(ctx, opts))
	root.AddCommand(searchCommand(ctx, opts))
	root.AddCommand(workspacesCommand(ctx, opts))
	root.AddCommand(dashboardsCommand(ctx, opts))
	root.AddCommand(semanticModelsCommand(ctx, opts))
	root.AddCommand(loginCommand(opts))
	root.AddCommand(adminCommand(ctx, opts))
	root.AddCommand(healthcheckCommand(ctx, opts))
	annotateCommandDocumentation(root)
	return root
}

func loginCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a LibreDash API token for a target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.target == "" || opts.token == "" {
				return fmt.Errorf("login requires --target and --token for v1 CLI authentication")
			}
			if _, err := targetEnvironment(cmd.Context(), http.DefaultClient, opts.target, opts.token, ""); err != nil {
				return err
			}
			config, err := loadClientConfig()
			if err != nil {
				return err
			}
			if config.Targets == nil {
				config.Targets = map[string]clientTarget{}
			}
			config.Targets[opts.target] = clientTarget{Token: opts.token}
			return saveClientConfig(config)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	return cmd
}
