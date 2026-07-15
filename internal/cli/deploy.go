package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"github.com/spf13/cobra"
)

type deployRequest struct {
	ProjectPath string
	Environment string
	Revisions   map[string]string
	Target      string
	Token       string
	AutoApprove bool
	Out         io.Writer
	HTTPClient  *http.Client
}

func deployCommand(ctx context.Context, opts *rootOptions) *cobra.Command {
	var revisions []string
	command := &cobra.Command{
		Use:   "deploy",
		Short: "Atomically deploy a configuration-as-code project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(opts.workspaceID) != "" {
				return fmt.Errorf("deploy is project-wide and does not accept --workspace")
			}
			target, token, err := clientTargetAndToken(opts)
			if err != nil {
				return err
			}
			pins, err := parseManagedRevisionPins(revisions)
			if err != nil {
				return err
			}
			return runDeploy(ctx, deployRequest{
				ProjectPath: opts.catalog, Environment: opts.environment, Revisions: pins,
				Target: target, Token: token, AutoApprove: opts.autoApprove,
				Out: cmd.OutOrStdout(), HTTPClient: http.DefaultClient,
			})
		},
	}
	command.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	command.Flags().StringVar(&opts.token, "token", "", "API token")
	command.Flags().StringVar(&opts.catalog, "project", filepath.Join("dashboards", "libredash.yaml"), "project path")
	command.Flags().StringVar(&opts.environment, "environment", "dev", "deployment environment")
	command.Flags().StringArrayVar(&revisions, "revision", nil, "managed revision pin as connection=sha256:<digest> (repeatable)")
	command.Flags().BoolVar(&opts.autoApprove, "auto-approve", false, "approve and activate the deployment without prompting")
	return command
}

func runDeploy(ctx context.Context, request deployRequest) error {
	request.ProjectPath = strings.TrimSpace(request.ProjectPath)
	request.Environment = strings.TrimSpace(request.Environment)
	request.Target = strings.TrimSpace(request.Target)
	request.Token = strings.TrimSpace(request.Token)
	if ctx == nil || request.ProjectPath == "" || request.Environment == "" || request.Target == "" || request.Token == "" {
		return fmt.Errorf("deploy requires project, environment, target, and token")
	}
	project, err := workspacecompiler.LoadProject(request.ProjectPath)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if _, err := workspacecompiler.CompileProject(request.ProjectPath, workspacecompiler.Options{ServingStateID: "deployment-preflight"}); err != nil {
		return fmt.Errorf("compile project: %w", err)
	}
	if err := validateManagedRevisionPins(project, request.Revisions); err != nil {
		return err
	}
	workspaceIDs := sortedProjectWorkspaceIDs(project.Workspaces)
	if len(workspaceIDs) == 0 {
		return fmt.Errorf("project %q has no workspaces", request.ProjectPath)
	}

	cliOpts := &rootOptions{
		target: request.Target, token: request.Token, catalog: request.ProjectPath,
		environment: request.Environment, autoApprove: request.AutoApprove,
	}
	type candidate struct {
		workspaceID string
		activeGraph workspace.AssetGraph
	}
	candidates := make([]candidate, 0, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		graph, graphErr := fetchActiveWorkspaceGraphFor(ctx, cliOpts, workspaceID)
		if graphErr != nil {
			return fmt.Errorf("read active graph for workspace %q", workspaceID)
		}
		plan, planErr := workspacecompiler.PlanProjectAgainstGraph(request.ProjectPath, workspaceID, graph)
		if planErr != nil {
			return fmt.Errorf("plan workspace %q: %w", workspaceID, planErr)
		}
		printDeploymentPlanSummaryTo(outputOrDiscard(request.Out), plan.Workspaces[0])
		candidates = append(candidates, candidate{workspaceID: workspaceID, activeGraph: graph})
	}
	if err := confirmDeployment(cliOpts, os.Stdin, outputOrDiscard(request.Out)); err != nil {
		return err
	}

	targets := make([]apigenapi.ProjectDeploymentTargetRequest, 0, len(candidates))
	for _, item := range candidates {
		workspaceProject := project.Workspaces[item.workspaceID]
		pins := selectManagedDataPins(request.Revisions, managedConnectionsForWorkspace(project, workspaceProject))
		prepared, localDigest, prepareErr := prepareWorkspaceCandidate(ctx, cliOpts, request.Target, request.Token, project.Name, item.workspaceID, workspaceProject, item.activeGraph, pins)
		if prepareErr != nil {
			return fmt.Errorf("prepare workspace %q failed", item.workspaceID)
		}
		if prepared.Digest != localDigest || !canonicalArtifactDigest(prepared.Digest) {
			return fmt.Errorf("prepare workspace %q returned an invalid artifact digest", item.workspaceID)
		}
		targets = append(targets, apigenapi.ProjectDeploymentTargetRequest{Workspace: item.workspaceID, CandidateId: prepared.Id})
	}

	client := newManagedDataCLIClient(request.HTTPClient, request.Target, request.Token)
	createBody := apigenapi.ProjectDeploymentCreateRequest{Environment: request.Environment, Targets: targets}
	createKeyValues := []string{project.Name, request.Environment}
	createKeyValues = append(createKeyValues, projectDeploymentTargetValues(targets)...)
	created, err := client.createProjectDeployment(ctx, project.Name, deploymentIdempotencyKey("create", createKeyValues...), createBody)
	if err != nil {
		return fmt.Errorf("create project deployment failed")
	}
	if err := validateProjectDeploymentResponse(created, "", project.Name, request.Environment, apigenapi.ProjectDeploymentStatusPending, apigenapi.ProjectDeploymentTargetStatusPending, targets); err != nil {
		return err
	}
	activated, err := client.activateProjectDeployment(ctx, project.Name, created.Id, deploymentIdempotencyKey("activate", project.Name, created.Id))
	if err != nil {
		return fmt.Errorf("activate project deployment failed")
	}
	if err := validateProjectDeploymentResponse(activated, created.Id, project.Name, request.Environment, apigenapi.ProjectDeploymentStatusActive, apigenapi.ProjectDeploymentTargetStatusActive, targets); err != nil {
		return err
	}
	_, err = fmt.Fprintf(outputOrDiscard(request.Out), "deployed %s deployment=%s environment=%s status=%s\n", project.Name, activated.Id, request.Environment, activated.Status)
	return err
}

func parseManagedRevisionPins(values []string) (map[string]string, error) {
	pins := make(map[string]string, len(values))
	for _, value := range values {
		name, revision, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		revision = strings.TrimSpace(revision)
		if !ok || name == "" || !canonicalManagedRevisionID(revision) {
			return nil, fmt.Errorf("revision must use connection=sha256:<64 lowercase hex>")
		}
		if _, duplicate := pins[name]; duplicate {
			return nil, fmt.Errorf("duplicate revision for managed connection %q", name)
		}
		pins[name] = revision
	}
	return pins, nil
}

func validateManagedRevisionPins(project workspacecompiler.Project, pins map[string]string) error {
	want := make(map[string]struct{})
	for _, workspaceProject := range project.Workspaces {
		for _, name := range managedConnectionsForWorkspace(project, workspaceProject) {
			want[name] = struct{}{}
		}
	}
	for name := range want {
		revision, ok := pins[name]
		if !ok {
			return fmt.Errorf("managed connection %q requires an explicit revision pin", name)
		}
		if !canonicalManagedRevisionID(revision) {
			return fmt.Errorf("managed connection %q revision must be canonical sha256:<64 lowercase hex>", name)
		}
	}
	for name := range pins {
		if _, ok := want[name]; !ok {
			return fmt.Errorf("revision provided for unknown managed connection %q", name)
		}
	}
	return nil
}

func printDeploymentPlanSummaryTo(out io.Writer, workspacePlan workspacecompiler.ProjectPlanWorkspace) {
	summary := workspacePlan.Summary
	fmt.Fprintf(out, "workspace %s changes +%d ~%d -%d dependencies %d\n", workspacePlan.ID, summary.Added, summary.Changed, summary.Removed, summary.DependencyChanges)
}

func projectDeploymentTargetValues(targets []apigenapi.ProjectDeploymentTargetRequest) []string {
	values := make([]string, 0, len(targets)*2)
	for _, target := range targets {
		values = append(values, target.Workspace, target.CandidateId)
	}
	return values
}

func validateProjectDeploymentResponse(response apigenapi.ProjectDeploymentResponse, expectedID, project, environment string, status apigenapi.ProjectDeploymentStatus, targetStatus apigenapi.ProjectDeploymentTargetStatus, targets []apigenapi.ProjectDeploymentTargetRequest) error {
	if strings.TrimSpace(response.Id) == "" || expectedID != "" && response.Id != expectedID || response.Project != project || response.Environment != environment || response.Status != status || len(response.Targets) != len(targets) {
		return fmt.Errorf("project deployment returned inconsistent scope or status")
	}
	expected := make(map[string]string, len(targets))
	for _, target := range targets {
		expected[target.Workspace] = target.CandidateId
	}
	for _, target := range response.Targets {
		if expected[target.Workspace] != target.CandidateId || target.Status != targetStatus {
			return fmt.Errorf("project deployment returned inconsistent targets")
		}
		delete(expected, target.Workspace)
	}
	if len(expected) != 0 {
		return fmt.Errorf("project deployment omitted targets")
	}
	return nil
}

func canonicalArtifactDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func deploymentIdempotencyKey(kind string, values ...string) string {
	digest := sha256.New()
	writeHashValue(digest, kind)
	for _, value := range values {
		writeHashValue(digest, value)
	}
	return "deployment-" + kind + "-" + hex.EncodeToString(digest.Sum(nil))
}

func outputOrDiscard(out io.Writer) io.Writer {
	if out == nil {
		return io.Discard
	}
	return out
}

func managedConnectionsForWorkspace(project workspacecompiler.Project, workspaceProject *workspacecompiler.WorkspaceProject) []string {
	connections := map[string]struct{}{}
	for sourceID := range workspaceProject.AllowedSources {
		source, ok := project.Sources[sourceID]
		if !ok {
			continue
		}
		connection, ok := project.Connections[source.Connection]
		if ok && connection.Kind == "managed" {
			connections[source.Connection] = struct{}{}
		}
	}
	result := make([]string, 0, len(connections))
	for connection := range connections {
		result = append(result, connection)
	}
	sort.Strings(result)
	return result
}

func selectManagedDataPins(all map[string]string, connections []string) map[string]string {
	pins := make(map[string]string, len(connections))
	for _, connection := range connections {
		pins[connection] = all[connection]
	}
	return pins
}

func canonicalManagedRevisionID(value string) bool {
	const prefix = "sha256:"
	if len(value) != len(prefix)+sha256.Size*2 || !strings.HasPrefix(value, prefix) {
		return false
	}
	digest := value[len(prefix):]
	if strings.ToLower(digest) != digest {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func sortedProjectWorkspaceIDs(workspaces map[string]*workspacecompiler.WorkspaceProject) []string {
	ids := make([]string, 0, len(workspaces))
	for id := range workspaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func prepareWorkspaceCandidate(ctx context.Context, opts *rootOptions, target, token, projectID, workspaceID string, workspaceProject *workspacecompiler.WorkspaceProject, activeGraph workspace.AssetGraph, managedDataRevisions map[string]string) (apigenapi.DeploymentCandidateResponse, string, error) {
	createBody, _ := json.Marshal(map[string]any{
		"title":       workspaceProject.Title,
		"description": workspaceProject.Description,
		"environment": cliEnvironment(opts),
	})
	var created apigenapi.DeploymentCandidateResponse
	candidatePathParams := map[string]string{"project": projectID, "workspace": workspaceID}
	createURL, err := apiOperationURL(target, "createDeploymentCandidate", candidatePathParams, nil)
	if err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPost, createURL, token, bytes.NewReader(createBody), &created); err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	uploadURL, err := apiOperationURL(target, "uploadDeploymentCandidateArtifact", map[string]string{"project": projectID, "workspace": workspaceID, "candidate": created.Id}, nil)
	if err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	digest, err := streamProjectArtifact(ctx, uploadURL, token, opts.catalog, servingstatefs.PackProjectOptions{
		WorkspaceID: workspaceID, Environment: servingstate.Environment(cliEnvironment(opts)), ServingStateID: servingstate.ID(created.Id),
		ActiveGraph: activeGraph, ManagedDataRevisions: managedDataRevisions,
	})
	if err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	var validated apigenapi.DeploymentCandidateResponse
	validateURL, err := apiOperationURL(target, "validateDeploymentCandidate", map[string]string{"project": projectID, "workspace": workspaceID, "candidate": created.Id}, nil)
	if err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	if err := doJSON(ctx, http.MethodPost, validateURL, token, nil, &validated); err != nil {
		return apigenapi.DeploymentCandidateResponse{}, "", err
	}
	if validated.Id != created.Id || validated.Project != projectID || validated.Workspace != workspaceID || validated.Environment != cliEnvironment(opts) || validated.Status != string(servingstate.StatusValidated) || strings.TrimSpace(validated.Digest) == "" {
		return apigenapi.DeploymentCandidateResponse{}, "", fmt.Errorf("workspace candidate validation returned inconsistent scope or status")
	}
	return validated, digest, nil
}

func streamProjectArtifact(ctx context.Context, uploadURL, token, projectPath string, options servingstatefs.PackProjectOptions) (string, error) {
	reader, writer := io.Pipe()
	type packResult struct {
		digest string
		err    error
	}
	result := make(chan packResult, 1)
	go func() {
		_, digest, err := servingstatefs.PackProject(projectPath, options, writer)
		_ = writer.CloseWithError(err)
		result <- packResult{digest: digest, err: err}
	}()
	uploadErr := doRawAPI(ctx, http.MethodPut, uploadURL, token, "application/gzip", reader, io.Discard)
	_ = reader.CloseWithError(uploadErr)
	packed := <-result
	if packed.err != nil {
		return "", packed.err
	}
	if uploadErr != nil {
		return "", uploadErr
	}
	return packed.digest, nil
}

func cliEnvironment(opts *rootOptions) string {
	if opts.environment == "" {
		return "dev"
	}
	return opts.environment
}

func confirmDeployment(opts *rootOptions, in *os.File, out io.Writer) error {
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
	fmt.Fprint(out, "Activate this project deployment? Type yes to continue: ")
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
