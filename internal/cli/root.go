package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	addr         string
	dataDir      string
	production   bool
	workspaceID  string
	environment  string
	target       string
	token        string
	catalog      string
	conversation string
	jsonOutput   bool
	pageID       string
	count        int
	filtersJSON  string
	bodyJSON     string
	schemaFormat string
	schemaOut    string
	limit        int
	pageToken    string
	searchTypes  []string
	autoApprove  bool
	apply        bool
}

func Execute(ctx context.Context) error {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:   "libredash",
		Short: "LibreDash BI-as-code server and publish CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(ctx, opts)
		},
	}
	root.PersistentFlags().StringVar(&opts.workspaceID, "workspace", "", "workspace id")
	root.AddCommand(serveCommand(ctx, opts))
	root.AddCommand(publishCommand(ctx, opts))
	root.AddCommand(validateCommand(ctx, opts))
	root.AddCommand(planCommand(ctx, opts))
	root.AddCommand(schemaCommand(opts))
	root.AddCommand(apiCommand(ctx, opts))
	root.AddCommand(agentCommand(ctx, opts))
	root.AddCommand(searchCommand(ctx, opts))
	root.AddCommand(workspacesCommand(ctx, opts))
	root.AddCommand(dashboardsCommand(ctx, opts))
	root.AddCommand(semanticModelsCommand(ctx, opts))
	root.AddCommand(loginCommand(opts))
	root.AddCommand(publishesCommand(ctx, opts))
	root.AddCommand(adminCommand(ctx, opts))
	return root.ExecuteContext(ctx)
}

func loginCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a LibreDash API token for a target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.target == "" || opts.token == "" {
				return fmt.Errorf("login requires --target and --token for v1 CLI authentication")
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
