package cli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacecompiler "github.com/Yacobolo/libredash/internal/workspace/compiler"
	"github.com/spf13/cobra"
)

type deployRequest struct {
	ProjectPath string
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
				ProjectPath: opts.catalog, Revisions: pins,
				Target: target, Token: token, AutoApprove: opts.autoApprove,
				Out: cmd.OutOrStdout(), HTTPClient: http.DefaultClient,
			})
		},
	}
	command.Flags().StringVar(&opts.target, "target", "", "LibreDash server URL")
	command.Flags().StringVar(&opts.token, "token", "", "API token")
	command.Flags().StringVar(&opts.catalog, "project", filepath.Join("dashboards", "libredash.yaml"), "project path")
	command.Flags().StringArrayVar(&revisions, "revision", nil, "managed revision pin as connection=sha256:<digest> (repeatable)")
	command.Flags().BoolVar(&opts.autoApprove, "auto-approve", false, "approve and activate the deployment without prompting")
	return command
}

func runDeploy(ctx context.Context, request deployRequest) error {
	request.ProjectPath = strings.TrimSpace(request.ProjectPath)
	request.Target = strings.TrimSpace(request.Target)
	request.Token = strings.TrimSpace(request.Token)
	if ctx == nil || request.ProjectPath == "" || request.Target == "" || request.Token == "" {
		return fmt.Errorf("deploy requires project, target, and token")
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

	client := newManagedDataCLIClient(request.HTTPClient, request.Target, request.Token)
	capabilities, err := client.capabilities(ctx)
	if err != nil || strings.TrimSpace(capabilities.Environment) == "" {
		return fmt.Errorf("read server capabilities failed")
	}
	cliOpts := &rootOptions{target: request.Target, token: request.Token, catalog: request.ProjectPath, autoApprove: request.AutoApprove}
	type plannedWorkspace struct {
		workspaceID string
		activeGraph workspace.AssetGraph
	}
	planned := make([]plannedWorkspace, 0, len(workspaceIDs))
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
		planned = append(planned, plannedWorkspace{workspaceID: workspaceID, activeGraph: graph})
	}
	if err := confirmDeployment(cliOpts, os.Stdin, outputOrDiscard(request.Out)); err != nil {
		return err
	}

	type artifact struct {
		workspaceID string
		digest      string
		content     []byte
	}
	artifacts := make([]artifact, 0, len(planned))
	projectDigest := ""
	for _, item := range planned {
		workspaceProject := project.Workspaces[item.workspaceID]
		pins := selectManagedDataPins(request.Revisions, managedConnectionsForWorkspace(project, workspaceProject))
		var content bytes.Buffer
		manifest, digest, packErr := servingstatefs.PackProject(request.ProjectPath, servingstatefs.PackProjectOptions{
			WorkspaceID: item.workspaceID, Environment: servingstate.Environment(capabilities.Environment), ServingStateID: "release-artifact",
			ActiveGraph: item.activeGraph, ManagedDataRevisions: pins,
		}, &content)
		if packErr != nil {
			return fmt.Errorf("package workspace %q: %w", item.workspaceID, packErr)
		}
		if projectDigest == "" {
			projectDigest = manifest.ProjectDigest
		} else if projectDigest != manifest.ProjectDigest {
			return fmt.Errorf("workspace %q produced an inconsistent project digest", item.workspaceID)
		}
		artifacts = append(artifacts, artifact{workspaceID: item.workspaceID, digest: digest, content: append([]byte(nil), content.Bytes()...)})
	}

	createBody := apigenapi.ReleaseCreateRequest{ProjectDigest: projectDigest, Workspaces: make([]apigenapi.ReleaseWorkspaceManifest, 0, len(artifacts)), Connections: []apigenapi.ReleaseConnectionPin{}}
	keyValues := []string{project.Name, projectDigest}
	for _, item := range artifacts {
		createBody.Workspaces = append(createBody.Workspaces, apigenapi.ReleaseWorkspaceManifest{Workspace: item.workspaceID, ArtifactDigest: item.digest})
		keyValues = append(keyValues, item.workspaceID, item.digest)
	}
	connectionIDs := make([]string, 0, len(request.Revisions))
	for connection := range request.Revisions {
		connectionIDs = append(connectionIDs, connection)
	}
	sort.Strings(connectionIDs)
	for _, connection := range connectionIDs {
		createBody.Connections = append(createBody.Connections, apigenapi.ReleaseConnectionPin{Connection: connection, RevisionId: request.Revisions[connection]})
		keyValues = append(keyValues, connection, request.Revisions[connection])
	}
	releaseKey := deploymentIdempotencyKey("release", keyValues...)
	created, err := client.createRelease(ctx, project.Name, releaseKey, createBody)
	if err != nil {
		return fmt.Errorf("create project release failed")
	}
	if created.ProjectId != project.Name || created.ProjectDigest != projectDigest || created.Id == "" {
		return fmt.Errorf("project release returned inconsistent scope or status")
	}
	finalized := created
	switch created.Status {
	case apigenapi.ReleaseStatusDraft:
		for _, item := range artifacts {
			digestBytes, _ := hex.DecodeString(item.digest)
			contentDigest := "sha-256=:" + base64.StdEncoding.EncodeToString(digestBytes) + ":"
			if _, err := client.uploadReleaseArtifact(ctx, project.Name, created.Id, item.workspaceID, contentDigest, bytes.NewReader(item.content)); err != nil {
				return fmt.Errorf("upload workspace %q artifact failed", item.workspaceID)
			}
		}
		finalized, err = client.finalizeRelease(ctx, project.Name, created.Id, deploymentIdempotencyKey("finalize", project.Name, created.Id))
		if err != nil {
			return fmt.Errorf("finalize project release failed")
		}
	case apigenapi.ReleaseStatusValidating:
		// Reissuing finalize is idempotent and restarts validation if a prior
		// server process stopped after persisting the validating state.
		finalized, err = client.finalizeRelease(ctx, project.Name, created.Id, deploymentIdempotencyKey("finalize", project.Name, created.Id))
		if err != nil {
			return fmt.Errorf("resume project release validation failed")
		}
	case apigenapi.ReleaseStatusReady:
	case apigenapi.ReleaseStatusFailed:
		return fmt.Errorf("existing project release validation failed; create a new release")
	default:
		return fmt.Errorf("project release returned unexpected status %q", created.Status)
	}
	finalized, err = waitForProjectRelease(ctx, client, project.Name, created.Id, finalized)
	if err != nil {
		return err
	}
	deployed, err := client.createDeployment(ctx, project.Name, deploymentIdempotencyKey("deploy", project.Name, created.Id), apigenapi.DeploymentCreateRequest{ReleaseId: created.Id})
	if err != nil {
		return fmt.Errorf("deploy project release failed")
	}
	if deployed.ProjectId != project.Name || deployed.ReleaseId != created.Id || deployed.Id == "" {
		return fmt.Errorf("project deployment returned inconsistent scope or status")
	}
	deployed, err = waitForProjectDeployment(ctx, client, project.Name, created.Id, deployed)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(outputOrDiscard(request.Out), "deployed %s release=%s deployment=%s environment=%s status=%s\n", project.Name, created.Id, deployed.Id, capabilities.Environment, deployed.Status)
	return err
}

func waitForProjectRelease(ctx context.Context, client *managedDataCLIClient, projectID, releaseID string, release apigenapi.ReleaseResponse) (apigenapi.ReleaseResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	for {
		if release.Id != releaseID || release.ProjectId != projectID {
			return apigenapi.ReleaseResponse{}, fmt.Errorf("project release returned inconsistent scope")
		}
		switch release.Status {
		case apigenapi.ReleaseStatusReady:
			return release, nil
		case apigenapi.ReleaseStatusValidating:
		case apigenapi.ReleaseStatusFailed:
			detail := ""
			if release.Error != nil {
				detail = ": " + *release.Error
			}
			return apigenapi.ReleaseResponse{}, fmt.Errorf("project release validation failed%s", detail)
		default:
			return apigenapi.ReleaseResponse{}, fmt.Errorf("project release validation returned unexpected status %q", release.Status)
		}
		select {
		case <-ctx.Done():
			return apigenapi.ReleaseResponse{}, fmt.Errorf("wait for project release validation: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
		next, err := client.getRelease(ctx, projectID, releaseID)
		if err != nil {
			return apigenapi.ReleaseResponse{}, fmt.Errorf("get project release failed")
		}
		release = next
	}
}

func waitForProjectDeployment(ctx context.Context, client *managedDataCLIClient, projectID, releaseID string, deployment apigenapi.DeploymentResponse) (apigenapi.DeploymentResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	for {
		switch deployment.Status {
		case apigenapi.DeploymentStatusActive:
			return deployment, nil
		case apigenapi.DeploymentStatusQueued, apigenapi.DeploymentStatusRunning:
		case apigenapi.DeploymentStatusFailed, apigenapi.DeploymentStatusCancelled:
			detail := ""
			if deployment.Error != nil {
				detail = ": " + *deployment.Error
			}
			return apigenapi.DeploymentResponse{}, fmt.Errorf("project deployment %s%s", deployment.Status, detail)
		default:
			return apigenapi.DeploymentResponse{}, fmt.Errorf("project deployment returned unexpected status %q", deployment.Status)
		}
		select {
		case <-ctx.Done():
			return apigenapi.DeploymentResponse{}, fmt.Errorf("wait for project deployment: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
		next, err := client.getDeployment(ctx, projectID, deployment.Id)
		if err != nil {
			return apigenapi.DeploymentResponse{}, fmt.Errorf("get project deployment failed")
		}
		if next.Id != deployment.Id || next.ProjectId != projectID || next.ReleaseId != releaseID {
			return apigenapi.DeploymentResponse{}, fmt.Errorf("project deployment returned inconsistent scope")
		}
		deployment = next
	}
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
