package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"github.com/spf13/cobra"
)

func validateCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [project]",
		Short: "Validate a configuration-as-code project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("validate accepts at most one positional project")
			}
			if len(args) == 1 {
				if cmd.Flags().Changed("project") {
					return fmt.Errorf("choose either --project or positional project, not both")
				}
				opts.catalog = args[0]
			}
			return runValidate(ctx, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.catalog, "project", filepath.Join("dashboards", "libredash.yaml"), "project path")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit JSON diagnostics")
	return cmd
}

func planCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan [project]",
		Short: "Emit a deterministic configuration-as-code plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("plan accepts at most one positional project")
			}
			if len(args) == 1 {
				if cmd.Flags().Changed("project") {
					return fmt.Errorf("choose either --project or positional project, not both")
				}
				opts.catalog = args[0]
			}
			return runPlan(ctx, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.catalog, "project", filepath.Join("dashboards", "libredash.yaml"), "project path")
	cmd.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL for active publish diff")
	cmd.Flags().StringVar(&opts.token, "token", "", "API token")
	cmd.Flags().StringVar(&opts.environment, "environment", "dev", "publish environment for active diff")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit JSON plan")
	return cmd
}

func schemaCommand(opts *rootOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "schema",
		Short: "Inspect LibreDash YAML schemas",
	}
	export := &cobra.Command{
		Use:   "export",
		Short: "Export generated schema artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaExport(opts)
		},
	}
	export.Flags().StringVar(&opts.schemaFormat, "format", "json-schema", "schema output format")
	export.Flags().StringVar(&opts.schemaOut, "out", filepath.Join("schemas", "json"), "output directory")
	parent.AddCommand(export)
	return parent
}

type validateResponse struct {
	OK          bool                      `json:"ok"`
	Diagnostics []configschema.Diagnostic `json:"diagnostics"`
}

func runValidate(ctx context.Context, opts *rootOptions, out io.Writer) error {
	diagnostics := validateProject(ctx, opts.catalog)
	response := validateResponse{OK: len(diagnostics) == 0, Diagnostics: diagnostics}
	if opts.jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			return err
		}
		if response.OK {
			return nil
		}
		return fmt.Errorf("validation failed")
	}
	if response.OK {
		fmt.Fprintf(out, "ok %s\n", opts.catalog)
		return nil
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(out, diagnostic.String())
	}
	return fmt.Errorf("validation failed")
}

func runPlan(ctx context.Context, opts *rootOptions, out io.Writer) error {
	var plan workspacecompiler.ProjectPlan
	var err error
	if opts.target != "" {
		if opts.workspaceID == "" {
			return fmt.Errorf("plan --target requires --workspace")
		}
		active, err := fetchActiveWorkspaceGraph(ctx, opts)
		if err != nil {
			return err
		}
		plan, err = workspacecompiler.PlanProjectAgainstGraph(opts.catalog, opts.workspaceID, active)
	} else {
		plan, err = workspacecompiler.PlanProject(opts.catalog)
	}
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	if err := renderProjectPlan(out, plan); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func renderProjectPlan(out io.Writer, plan workspacecompiler.ProjectPlan) error {
	fmt.Fprintf(out, "project %s\n", plan.Project)
	for _, workspace := range plan.Workspaces {
		fmt.Fprintf(out, "workspace %s\n", workspace.ID)
		fmt.Fprintf(out, "  connections %s\n", strings.Join(workspace.Connections, ","))
		fmt.Fprintf(out, "  sources %s\n", strings.Join(workspace.Sources, ","))
		fmt.Fprintf(out, "  model_tables %s\n", strings.Join(workspace.ModelTables, ","))
		fmt.Fprintf(out, "  semantic_models %s\n", strings.Join(workspace.SemanticModels, ","))
		fmt.Fprintf(out, "  dashboards %s\n", strings.Join(workspace.Dashboards, ","))
		fmt.Fprintf(out, "  workspace_groups %s\n", strings.Join(workspace.WorkspaceGroups, ","))
		fmt.Fprintf(out, "  workspace_role_bindings %s\n", strings.Join(workspace.WorkspaceRoleBindings, ","))
		fmt.Fprintf(out, "  workspace_agent_policies %s\n", strings.Join(workspace.WorkspaceAgentPolicies, ","))
		if len(workspace.Changes) > 0 || len(workspace.DependencyChanges) > 0 {
			fmt.Fprintf(out, "  changes +%d ~%d -%d dependencies %d\n", workspace.Summary.Added, workspace.Summary.Changed, workspace.Summary.Removed, workspace.Summary.DependencyChanges)
			for _, change := range workspace.Changes {
				fmt.Fprintf(out, "    %s %s", change.Action, change.ID)
				annotations := planChangeAnnotations(change)
				if annotations != "" {
					fmt.Fprintf(out, " [%s]", annotations)
				}
				fmt.Fprintln(out)
			}
			for _, change := range workspace.DependencyChanges {
				fmt.Fprintf(out, "    %s dependency %s -> %s (%s)", change.Action, change.From, change.To, change.Type)
				if change.Breaking {
					fmt.Fprint(out, " [breaking]")
				}
				fmt.Fprintln(out)
			}
		}
	}
	return nil
}

func fetchActiveWorkspaceGraph(ctx context.Context, opts *rootOptions) (workspace.AssetGraph, error) {
	return fetchActiveWorkspaceGraphFor(ctx, opts, opts.workspaceID)
}

func fetchActiveWorkspaceGraphFor(ctx context.Context, opts *rootOptions, workspaceID string) (workspace.AssetGraph, error) {
	target, token, err := clientTargetAndToken(opts)
	if err != nil {
		return workspace.AssetGraph{}, err
	}
	query := url.Values{}
	endpoint, err := apiOperationURL(target, "getWorkspaceActiveAssetGraph", map[string]string{"workspace": workspaceID}, withEnvironmentQuery(cliEnvironment(opts), query))
	if err != nil {
		return workspace.AssetGraph{}, err
	}
	var response api.WorkspaceAssetGraphResponse
	if err := doJSON(ctx, http.MethodGet, endpoint, token, nil, &response); err != nil {
		return workspace.AssetGraph{}, err
	}
	graph := workspace.AssetGraph{
		Assets: make([]workspace.Asset, 0, len(response.Assets)),
		Edges:  make([]workspace.AssetEdge, 0, len(response.Edges)),
	}
	for _, asset := range response.Assets {
		graph.Assets = append(graph.Assets, workspace.Asset{
			ID:             workspace.AssetID(asset.ID),
			SnapshotID:     workspace.AssetSnapshotID(asset.SnapshotID),
			WorkspaceID:    workspace.WorkspaceID(asset.WorkspaceID),
			ServingStateID: workspace.ServingStateID(asset.ServingStateID),
			Type:           workspace.AssetType(asset.Type),
			Key:            asset.Key,
			ParentID:       workspace.AssetID(asset.ParentID),
			Title:          asset.Title,
			Description:    asset.Description,
			PayloadSchema:  asset.PayloadSchema,
			SourceFile:     asset.SourceFile,
			PayloadJSON:    assetPayloadJSON(asset.Payload),
			ContentHash:    asset.ContentHash,
		})
	}
	for _, edge := range response.Edges {
		graph.Edges = append(graph.Edges, workspace.AssetEdge{
			ID:             workspace.AssetEdgeID(edge.ID),
			WorkspaceID:    workspace.WorkspaceID(edge.WorkspaceID),
			ServingStateID: workspace.ServingStateID(edge.ServingStateID),
			FromAssetID:    workspace.AssetID(edge.FromAssetID),
			ToAssetID:      workspace.AssetID(edge.ToAssetID),
			Type:           workspace.AssetEdgeType(edge.Type),
		})
	}
	return graph, nil
}

func assetPayloadJSON(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func environmentQuery(opts *rootOptions, values url.Values) url.Values {
	return withEnvironmentQuery(cliEnvironment(opts), values)
}

func withEnvironmentQuery(environment string, values url.Values) url.Values {
	if values == nil {
		values = url.Values{}
	}
	if environment == "" {
		environment = "dev"
	}
	values.Set("environment", environment)
	return values
}

func planChangeAnnotations(change workspacecompiler.ProjectPlanChange) string {
	parts := []string{}
	if change.Breaking {
		parts = append(parts, "breaking")
	}
	if change.MaterializationImpact {
		parts = append(parts, "refresh")
	}
	if change.AccessImpact {
		parts = append(parts, "access")
	}
	if change.AgentPolicyImpact {
		parts = append(parts, "agent_policy")
	}
	return strings.Join(parts, ",")
}

func validateProject(ctx context.Context, projectPath string) []configschema.Diagnostic {
	if _, err := workspacecompiler.CompileProject(projectPath, workspacecompiler.Options{}); err != nil {
		return configschema.Diagnostics(err)
	}
	if err := ctx.Err(); err != nil {
		return configschema.Diagnostics(err)
	}
	return nil
}

func runSchemaExport(opts *rootOptions) error {
	if opts.schemaFormat != "json-schema" {
		return fmt.Errorf("unsupported schema format %q", opts.schemaFormat)
	}
	files, err := configschema.JSONSchemaFiles()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.schemaOut, 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(opts.schemaOut, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
