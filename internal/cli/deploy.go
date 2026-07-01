package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/deployment"
	deploymentfs "github.com/Yacobolo/libredash/internal/deployment/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"github.com/spf13/cobra"
)

func deployCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a configuration-as-code project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.catalog, "project", filepath.Join("dashboards", "libredash.yaml"), "project path")
	cmd.Flags().StringVar(&opts.environment, "environment", "dev", "deployment environment")
	cmd.Flags().BoolVar(&opts.autoApprove, "auto-approve", false, "approve and activate the deployment without prompting")
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
	list.Flags().StringVar(&opts.environment, "environment", "dev", "deployment environment")
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
	rollback.Flags().StringVar(&opts.environment, "environment", "dev", "deployment environment")
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
	cmd.Flags().StringVar(&opts.environment, "environment", "dev", "deployment environment")
	return cmd
}

func runDeploy(ctx context.Context, opts *rootOptions) error {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	project, err := workspacecompiler.LoadProject(opts.catalog)
	if err != nil {
		return err
	}
	workspaceIDs := sortedDeployWorkspaceIDs(project.Workspaces, opts.workspaceID)
	if len(workspaceIDs) == 0 {
		if opts.workspaceID != "" {
			return fmt.Errorf("project %q has no workspace %q", opts.catalog, opts.workspaceID)
		}
		return fmt.Errorf("project %q has no workspaces", opts.catalog)
	}

	results := make([]deployWorkspaceResult, 0, len(workspaceIDs))
	needsApproval := false
	for _, workspaceID := range workspaceIDs {
		result := deployWorkspaceResult{WorkspaceID: workspaceID}
		activeGraph, err := fetchActiveWorkspaceGraphFor(ctx, opts, workspaceID)
		if err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}
		plan, err := workspacecompiler.PlanProjectAgainstGraph(opts.catalog, workspaceID, activeGraph)
		if err != nil {
			result.Err = err
			results = append(results, result)
			continue
		}
		workspacePlan := plan.Workspaces[0]
		if len(workspaceIDs) == 1 {
			if err := renderProjectPlan(os.Stdout, plan); err != nil {
				return err
			}
		} else {
			printDeployPlanSummary(workspacePlan)
		}
		if projectPlanWorkspaceUnchanged(workspacePlan) {
			result.Status = "skipped"
			results = append(results, result)
			continue
		}
		result.Status = "pending"
		result.ActiveGraph = activeGraph
		needsApproval = true
		results = append(results, result)
	}
	if needsApproval {
		if err := confirmDeploy(opts, os.Stdin, os.Stdout); err != nil {
			return err
		}
	}

	var failures []string
	for index := range results {
		result := &results[index]
		if result.Err != nil {
			result.Status = "failed"
			failures = append(failures, fmt.Sprintf("%s: %v", result.WorkspaceID, result.Err))
			fmt.Printf("failed %s: %v\n", result.WorkspaceID, result.Err)
			continue
		}
		if result.Status == "skipped" {
			fmt.Printf("skipped %s unchanged\n", result.WorkspaceID)
			continue
		}
		workspaceProject := project.Workspaces[result.WorkspaceID]
		activated, digest, err := deployWorkspace(ctx, opts, target, token, result.WorkspaceID, workspaceProject, result.ActiveGraph)
		if err != nil {
			result.Status = "failed"
			failures = append(failures, fmt.Sprintf("%s: %v", result.WorkspaceID, err))
			fmt.Printf("failed %s: %v\n", result.WorkspaceID, err)
			continue
		}
		result.Status = "deployed"
		fmt.Printf("deployed %s deployment=%s environment=%s digest=%s localDigest=%s status=%s\n", result.WorkspaceID, activated.ID, activated.Environment, activated.Digest, digest, activated.Status)
	}
	if len(failures) > 0 {
		return fmt.Errorf("deploy failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

type deployWorkspaceResult struct {
	WorkspaceID string
	Status      string
	ActiveGraph workspace.AssetGraph
	Err         error
}

func sortedDeployWorkspaceIDs(workspaces map[string]*workspacecompiler.WorkspaceProject, filter string) []string {
	if filter != "" {
		if _, ok := workspaces[filter]; !ok {
			return nil
		}
		return []string{filter}
	}
	ids := make([]string, 0, len(workspaces))
	for id := range workspaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func projectPlanWorkspaceUnchanged(workspacePlan workspacecompiler.ProjectPlanWorkspace) bool {
	return workspacePlan.Summary.Added == 0 &&
		workspacePlan.Summary.Changed == 0 &&
		workspacePlan.Summary.Removed == 0 &&
		workspacePlan.Summary.DependencyChanges == 0 &&
		len(workspacePlan.Changes) == 0 &&
		len(workspacePlan.DependencyChanges) == 0
}

func printDeployPlanSummary(workspacePlan workspacecompiler.ProjectPlanWorkspace) {
	summary := workspacePlan.Summary
	fmt.Printf("workspace %s changes +%d ~%d -%d dependencies %d\n", workspacePlan.ID, summary.Added, summary.Changed, summary.Removed, summary.DependencyChanges)
}

func deployWorkspace(ctx context.Context, opts *rootOptions, target, token, workspaceID string, workspaceProject *workspacecompiler.WorkspaceProject, activeGraph workspace.AssetGraph) (api.DeploymentResponse, string, error) {
	createBody, _ := json.Marshal(map[string]any{
		"title":       workspaceProject.Title,
		"description": workspaceProject.Description,
		"environment": cliEnvironment(opts),
	})
	var created api.DeploymentResponse
	workspacePathParams := map[string]string{"workspace": workspaceID}
	createURL, err := apiOperationURL(target, "createDeployment", workspacePathParams, environmentQuery(opts, nil))
	if err != nil {
		return api.DeploymentResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPost, createURL, token, bytes.NewReader(createBody), &created); err != nil {
		return api.DeploymentResponse{}, "", err
	}
	var buf bytes.Buffer
	var digest string
	_, digest, err = deploymentfs.PackProjectAgainstGraphForEnvironment(opts.catalog, workspaceID, deployment.Environment(cliEnvironment(opts)), deployment.ID(created.ID), activeGraph, &buf)
	if err != nil {
		return api.DeploymentResponse{}, "", err
	}
	uploadURL, err := apiOperationURL(target, "uploadDeploymentArtifact", map[string]string{"workspace": workspaceID, "deployment": created.ID}, environmentQuery(opts, nil))
	if err != nil {
		return api.DeploymentResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPut, uploadURL, token, bytes.NewReader(buf.Bytes()), nil); err != nil {
		return api.DeploymentResponse{}, "", err
	}
	var validated api.DeploymentResponse
	validateURL, err := apiOperationURL(target, "validateDeployment", map[string]string{"workspace": workspaceID, "deployment": created.ID}, environmentQuery(opts, nil))
	if err != nil {
		return api.DeploymentResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPost, validateURL, token, nil, &validated); err != nil {
		return api.DeploymentResponse{}, "", err
	}
	var activated api.DeploymentResponse
	activateURL, err := apiOperationURL(target, "activateDeployment", map[string]string{"workspace": workspaceID, "deployment": created.ID}, environmentQuery(opts, nil))
	if err != nil {
		return api.DeploymentResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPost, activateURL, token, nil, &activated); err != nil {
		return api.DeploymentResponse{}, "", err
	}
	return activated, digest, nil
}

func runDeploymentsList(ctx context.Context, opts *rootOptions) error {
	if opts.workspaceID == "" {
		return fmt.Errorf("deployments list requires --workspace")
	}
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	listURL, err := apiOperationURL(target, "listDeployments", map[string]string{"workspace": opts.workspaceID}, environmentQuery(opts, paginationQuery(opts)))
	if err != nil {
		return err
	}
	var response apiListResponse[api.DeploymentResponse]
	if err := doJSON(ctx, http.MethodGet, listURL, token, nil, &response); err != nil {
		return err
	}
	rows := response.Items
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tENVIRONMENT\tSTATUS\tDIGEST\tCREATED\tACTIVATED")
	for _, row := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", row.ID, row.Environment, row.Status, shortDigest(row.Digest), row.CreatedAt, row.ActivatedAt)
	}
	return tw.Flush()
}

func runRollback(ctx context.Context, opts *rootOptions) error {
	if opts.deployment == "" {
		return fmt.Errorf("rollback requires --deployment")
	}
	if opts.workspaceID == "" {
		return fmt.Errorf("rollback requires --workspace")
	}
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return err
	}
	var row api.DeploymentResponse
	rollbackURL, err := apiOperationURL(target, "activateDeployment", map[string]string{"workspace": opts.workspaceID, "deployment": opts.deployment}, environmentQuery(opts, nil))
	if err != nil {
		return err
	}
	if err := doJSON(ctx, http.MethodPost, rollbackURL, token, nil, &row); err != nil {
		return err
	}
	fmt.Printf("activated %s environment=%s status=%s\n", row.ID, row.Environment, row.Status)
	return nil
}

func cliEnvironment(opts *rootOptions) string {
	if opts.environment == "" {
		return "dev"
	}
	return opts.environment
}

func confirmDeploy(opts *rootOptions, in *os.File, out io.Writer) error {
	if opts.autoApprove {
		return nil
	}
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("deploy requires --auto-approve when stdin is not interactive")
	}
	fmt.Fprint(out, "Activate this deployment? Type yes to continue: ")
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("deploy requires --auto-approve when stdin is not interactive")
		}
		return err
	}
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		return fmt.Errorf("deployment activation cancelled")
	}
	return nil
}
