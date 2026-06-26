package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/Yacobolo/libredash/internal/api"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/spf13/cobra"
)

func deployCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a dashboard-as-code catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.catalog, "catalog", filepath.Join("dashboards", "catalog.yaml"), "catalog path")
	return cmd
}

func deploymentsCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{Use: "deployments", Short: "Inspect deployments"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List deployments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploymentsList(ctx, opts)
		},
	}
	list.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	list.Flags().StringVar(&opts.token, "token", "", "API token")
	addPaginationFlags(list, opts)
	rollback := &cobra.Command{
		Use:   "rollback",
		Short: "Activate a previous deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(ctx, opts)
		},
	}
	rollback.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	rollback.Flags().StringVar(&opts.token, "token", "", "API token")
	rollback.Flags().StringVar(&opts.deployment, "deployment", "", "deployment id")
	parent.AddCommand(list, rollback)
	return parent
}

func rollbackCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Activate a previous deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRollback(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.deployment, "deployment", "", "deployment id")
	return cmd
}

func runDeploy(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	manifest, digest, err := deploymentfs.PackCatalog(opts.catalog, &buf)
	if err != nil {
		return err
	}
	createBody, _ := json.Marshal(map[string]any{
		"title": manifest.WorkspaceTitle,
	})
	var created api.DeploymentResponse
	workspacePathParams := map[string]string{"workspace": opts.workspaceID}
	createURL, err := apiOperationURL(target, "createDeployment", workspacePathParams, nil)
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPost, createURL, token, bytes.NewReader(createBody), &created); err != nil {
		return err
	}
	uploadURL, err := apiOperationURL(target, "uploadDeploymentArtifact", map[string]string{"workspace": opts.workspaceID, "deployment": created.ID}, nil)
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPut, uploadURL, token, bytes.NewReader(buf.Bytes()), nil); err != nil {
		return err
	}
	var validated api.DeploymentResponse
	validateURL, err := apiOperationURL(target, "validateDeployment", map[string]string{"workspace": opts.workspaceID, "deployment": created.ID}, nil)
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPost, validateURL, token, nil, &validated); err != nil {
		return err
	}
	var activated api.DeploymentResponse
	activateURL, err := apiOperationURL(target, "activateDeployment", map[string]string{"workspace": opts.workspaceID, "deployment": created.ID}, nil)
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPost, activateURL, token, nil, &activated); err != nil {
		return err
	}
	fmt.Printf("deployed %s digest=%s localDigest=%s status=%s\n", activated.ID, activated.Digest, digest, activated.Status)
	return nil
}

func runDeploymentsList(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	listURL, err := apiOperationURL(target, "listDeployments", map[string]string{"workspace": opts.workspaceID}, paginationQuery(opts))
	if err != nil {
		return err
	}
	var response apiListResponse[api.DeploymentResponse]
	if err := doJSON(ctx, http.MethodGet, listURL, token, nil, &response); err != nil {
		return err
	}
	rows := response.Items
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tDIGEST\tCREATED\tACTIVATED")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.ID, row.Status, shortDigest(row.Digest), row.CreatedAt, row.ActivatedAt)
	}
	return tw.Flush()
}

func runRollback(ctx context.Context, opts *rootOptions) error {
	if opts.deployment == "" {
		return fmt.Errorf("rollback requires --deployment")
	}
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var row api.DeploymentResponse
	rollbackURL, err := apiOperationURL(target, "activateDeployment", map[string]string{"workspace": opts.workspaceID, "deployment": opts.deployment}, nil)
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPost, rollbackURL, token, nil, &row); err != nil {
		return err
	}
	fmt.Printf("activated %s status=%s\n", row.ID, row.Status)
	return nil
}
