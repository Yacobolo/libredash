package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/deployment"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB)}
}

func (r *Repository) Create(ctx context.Context, input deployment.CreateInput) (deployment.Deployment, error) {
	id := deployment.ID(newID("dep"))
	if err := r.q.CreateDeployment(ctx, platformdb.CreateDeploymentParams{
		ID:          string(id),
		WorkspaceID: string(input.WorkspaceID),
		Environment: string(deployment.NormalizeEnvironment(input.Environment)),
		Status:      string(deployment.StatusPending),
		Source:      string(deployment.NormalizeSource(input.Source)),
		CreatedBy:   input.CreatedBy,
	}); err != nil {
		return deployment.Deployment{}, err
	}
	return r.ByID(ctx, id)
}

func (r *Repository) ByID(ctx context.Context, id deployment.ID) (deployment.Deployment, error) {
	row, err := r.q.GetDeployment(ctx, string(id))
	if err != nil {
		return deployment.Deployment{}, mapNotFound(err)
	}
	return mapDeployment(row), nil
}

func (r *Repository) List(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) ([]deployment.Deployment, error) {
	rows, err := r.q.ListDeployments(ctx, platformdb.ListDeploymentsParams{WorkspaceID: string(workspaceID), Environment: string(deployment.NormalizeEnvironment(environment))})
	if err != nil {
		return nil, err
	}
	deployments := make([]deployment.Deployment, 0, len(rows))
	for _, row := range rows {
		deployments = append(deployments, mapDeployment(row))
	}
	return deployments, nil
}

func (r *Repository) MarkFailed(ctx context.Context, deploymentID deployment.ID, cause error) error {
	if cause == nil {
		return nil
	}
	return r.q.UpdateDeploymentStatus(ctx, platformdb.UpdateDeploymentStatusParams{
		Status: string(deployment.StatusFailed),
		Error:  cause.Error(),
		ID:     string(deploymentID),
	})
}

func (r *Repository) RecordDuckLakeSnapshot(ctx context.Context, deploymentID deployment.ID, snapshotID int64) error {
	if snapshotID <= 0 {
		return fmt.Errorf("ducklake snapshot id must be positive")
	}
	return r.q.UpdateDeploymentDuckLakeSnapshot(ctx, platformdb.UpdateDeploymentDuckLakeSnapshotParams{
		DucklakeSnapshotID: snapshotID,
		ID:                 string(deploymentID),
	})
}

func (r *Repository) ReferencedDuckLakeSnapshots(ctx context.Context) ([]int64, error) {
	return r.q.ListReferencedDuckLakeSnapshots(ctx)
}

func (r *Repository) ActiveDuckLakeSnapshots(ctx context.Context) ([]int64, error) {
	return r.q.ListActiveDuckLakeSnapshots(ctx)
}

func (r *Repository) LeasedDuckLakeSnapshots(ctx context.Context) ([]int64, error) {
	return r.q.ListLeasedDuckLakeSnapshots(ctx)
}

func (r *Repository) CreateQuerySnapshotLease(ctx context.Context, input deployment.SnapshotLeaseInput) (string, error) {
	if input.WorkspaceID == "" {
		return "", fmt.Errorf("workspace id is required")
	}
	if input.DeploymentID == "" {
		return "", fmt.Errorf("deployment id is required")
	}
	if input.DuckLakeSnapshotID <= 0 {
		return "", fmt.Errorf("ducklake snapshot id must be positive")
	}
	expiresAt := input.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(5 * time.Minute)
	}
	id := newID("lease")
	if err := r.q.CreateQuerySnapshotLease(ctx, platformdb.CreateQuerySnapshotLeaseParams{
		ID:                 id,
		WorkspaceID:        string(input.WorkspaceID),
		Environment:        string(deployment.NormalizeEnvironment(input.Environment)),
		DeploymentID:       string(input.DeploymentID),
		DucklakeSnapshotID: input.DuckLakeSnapshotID,
		OwnerID:            input.OwnerID,
		ExpiresAt:          sqliteTimestamp(expiresAt),
	}); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) ReleaseQuerySnapshotLease(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	return r.q.ReleaseQuerySnapshotLease(ctx, id)
}

func (r *Repository) ExtendQuerySnapshotLease(ctx context.Context, id string, expiresAt time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if expiresAt.IsZero() {
		return fmt.Errorf("lease expiry is required")
	}
	return r.q.ExtendQuerySnapshotLease(ctx, platformdb.ExtendQuerySnapshotLeaseParams{
		ID:        id,
		ExpiresAt: sqliteTimestamp(expiresAt),
	})
}

func (r *Repository) ReleaseExpiredQuerySnapshotLeases(ctx context.Context) error {
	return r.q.ReleaseExpiredQuerySnapshotLeases(ctx)
}

func (r *Repository) ExpireInactiveDeployments(ctx context.Context) error {
	return r.q.ExpireInactiveDeployments(ctx)
}

func (r *Repository) ScheduleExpiredDeploymentDeletion(ctx context.Context) error {
	return r.q.ScheduleExpiredDeploymentDeletion(ctx)
}

func (r *Repository) MarkDeleteScheduledDeploymentsDeleted(ctx context.Context) error {
	return r.q.MarkDeleteScheduledDeploymentsDeleted(ctx)
}

func (r *Repository) ReconcileRetention(ctx context.Context, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	if err := r.q.MarkDrainingDeploymentsDeleteScheduled(ctx); err != nil {
		return err
	}
	return r.q.MarkDeleteScheduledDeploymentsDeleted(ctx)
}

func (r *Repository) SaveValidated(ctx context.Context, deploymentID deployment.ID, validation deployment.Validation, artifact deployment.Artifact) (deployment.Deployment, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return deployment.Deployment{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	artifact.DeploymentID = deploymentID
	current, err := q.GetDeployment(ctx, string(deploymentID))
	if err != nil {
		return deployment.Deployment{}, mapNotFound(err)
	}
	if artifact.WorkspaceID != deployment.WorkspaceID(current.WorkspaceID) {
		return deployment.Deployment{}, fmt.Errorf("artifact workspace = %q, want %q", artifact.WorkspaceID, current.WorkspaceID)
	}
	if deployment.NormalizeEnvironment(artifact.Environment) != deployment.Environment(current.Environment) {
		return deployment.Deployment{}, fmt.Errorf("artifact environment = %q, want %q", deployment.NormalizeEnvironment(artifact.Environment), current.Environment)
	}
	if err := workspace.ValidateAssetGraphForDeployment(validation.Graph, workspace.WorkspaceID(current.WorkspaceID), workspace.DeploymentID(deploymentID)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.InsertDeploymentArtifact(ctx, mapArtifactParams(artifact)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.ClearAssetEdgesForDeployment(ctx, string(deploymentID)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.ClearAssetsForDeployment(ctx, string(deploymentID)); err != nil {
		return deployment.Deployment{}, err
	}
	for _, asset := range validation.Graph.Assets {
		if err := q.InsertAsset(ctx, platformdb.InsertAssetParams{
			SnapshotID:           string(asset.SnapshotID),
			LogicalAssetID:       string(asset.ID),
			WorkspaceID:          string(asset.WorkspaceID),
			DeploymentID:         string(asset.DeploymentID),
			AssetType:            string(asset.Type),
			AssetKey:             asset.Key,
			ParentLogicalAssetID: string(asset.ParentID),
			Title:                asset.Title,
			Description:          asset.Description,
			SourceFile:           asset.SourceFile,
			PayloadSchema:        asset.PayloadSchema,
			PayloadJson:          asset.PayloadJSON,
			ContentHash:          asset.ContentHash,
		}); err != nil {
			return deployment.Deployment{}, err
		}
	}
	for _, edge := range validation.Graph.Edges {
		if err := q.InsertAssetEdge(ctx, platformdb.InsertAssetEdgeParams{
			ID:                 string(edge.ID),
			WorkspaceID:        string(edge.WorkspaceID),
			DeploymentID:       string(edge.DeploymentID),
			FromLogicalAssetID: string(edge.FromAssetID),
			ToLogicalAssetID:   string(edge.ToAssetID),
			EdgeType:           string(edge.Type),
		}); err != nil {
			return deployment.Deployment{}, err
		}
	}
	if err := q.UpdateDeploymentValidated(ctx, platformdb.UpdateDeploymentValidatedParams{
		Status:       string(deployment.StatusValidated),
		Digest:       validation.Digest,
		ManifestJson: validation.ManifestJSON,
		ID:           string(deploymentID),
	}); err != nil {
		return deployment.Deployment{}, err
	}
	if err := tx.Commit(); err != nil {
		return deployment.Deployment{}, err
	}
	return r.ByID(ctx, deploymentID)
}

func (r *Repository) Activate(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID) (deployment.Deployment, error) {
	return r.activate(ctx, workspaceID, environment, deploymentID, nil)
}

func (r *Repository) ActivateWithWorkspacePolicy(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID, policy workspace.AccessPolicy) (deployment.Deployment, error) {
	return r.activate(ctx, workspaceID, environment, deploymentID, &policy)
}

func (r *Repository) activate(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment, deploymentID deployment.ID, policy *workspace.AccessPolicy) (deployment.Deployment, error) {
	environment = deployment.NormalizeEnvironment(environment)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return deployment.Deployment{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetDeployment(ctx, string(deploymentID))
	if err != nil {
		return deployment.Deployment{}, mapNotFound(err)
	}
	current := mapDeployment(row)
	if current.WorkspaceID != workspaceID {
		return deployment.Deployment{}, fmt.Errorf("deployment %s is not in workspace %s", deploymentID, workspaceID)
	}
	if current.Environment != environment {
		return deployment.Deployment{}, fmt.Errorf("deployment %s environment = %q, want %q", deploymentID, current.Environment, environment)
	}
	if !current.CanActivate() {
		return deployment.Deployment{}, fmt.Errorf("deployment %s has status %q, want validated", deploymentID, current.Status)
	}
	if policy != nil {
		if err := reconcileWorkspacePolicyTx(ctx, q, string(workspaceID), *policy); err != nil {
			return deployment.Deployment{}, err
		}
	}
	if err := q.MarkOtherDeploymentsDraining(ctx, platformdb.MarkOtherDeploymentsDrainingParams{
		WorkspaceID: string(workspaceID),
		Environment: string(environment),
		ID:          string(deploymentID),
	}); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.MarkDeploymentActive(ctx, string(deploymentID)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.SetActiveDeployment(ctx, platformdb.SetActiveDeploymentParams{
		WorkspaceID:  string(workspaceID),
		Environment:  string(environment),
		DeploymentID: string(deploymentID),
	}); err != nil {
		return deployment.Deployment{}, err
	}
	if err := tx.Commit(); err != nil {
		return deployment.Deployment{}, err
	}
	return r.ByID(ctx, deploymentID)
}

func reconcileWorkspacePolicyTx(ctx context.Context, q *platformdb.Queries, workspaceID string, policy workspace.AccessPolicy) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspace id is required")
	}
	bindings, err := q.ListRoleBindingsByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := q.DeleteRoleBindingByID(ctx, platformdb.DeleteRoleBindingByIDParams{WorkspaceID: workspaceID, ID: binding.ID}); err != nil {
			return err
		}
	}
	groups, err := q.ListGroupsByWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if err := q.DeleteGroup(ctx, platformdb.DeleteGroupParams{WorkspaceID: workspaceID, ID: group.ID}); err != nil {
			return err
		}
	}

	groupIDs := map[string]string{}
	for _, name := range sortedWorkspaceGroupNames(policy.Groups) {
		group := policy.Groups[name]
		id := stableAccessID("group", workspaceID, name)
		if err := q.UpsertGroup(ctx, platformdb.UpsertGroupParams{
			ID:          id,
			WorkspaceID: workspaceID,
			Provider:    "local",
			ExternalID:  name,
			Name:        firstNonEmpty(group.Name, name),
		}); err != nil {
			return err
		}
		groupIDs[name] = id
		for _, member := range group.Members {
			principalID, err := upsertPolicyPrincipalTx(ctx, q, member.PrincipalID, member.Email, member.DisplayName)
			if err != nil {
				return err
			}
			if err := q.InsertGroupMember(ctx, platformdb.InsertGroupMemberParams{WorkspaceID: workspaceID, GroupID: id, PrincipalID: principalID}); err != nil {
				return err
			}
		}
	}

	for _, name := range sortedWorkspaceRoleBindingNames(policy.RoleBindings) {
		binding := policy.RoleBindings[name]
		role, err := q.GetRoleByName(ctx, binding.Role)
		if err != nil {
			return err
		}
		params := platformdb.InsertRoleBindingParams{
			ID:          stableAccessID("rolebinding", workspaceID, name),
			WorkspaceID: workspaceID,
			RoleID:      role.ID,
		}
		switch binding.Subject.Kind {
		case string(access.SubjectGroup):
			groupID := groupIDs[binding.Subject.Group]
			if groupID == "" {
				return fmt.Errorf("workspace role binding %q references unknown group %q", name, binding.Subject.Group)
			}
			params.GroupID = sql.NullString{String: groupID, Valid: true}
		case string(access.SubjectPrincipal):
			principalID, err := upsertPolicyPrincipalTx(ctx, q, binding.Subject.PrincipalID, binding.Subject.Email, binding.Subject.DisplayName)
			if err != nil {
				return err
			}
			params.PrincipalID = sql.NullString{String: principalID, Valid: true}
		default:
			return fmt.Errorf("workspace role binding %q has unsupported subject kind %q", name, binding.Subject.Kind)
		}
		if err := q.InsertRoleBinding(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

func upsertPolicyPrincipalTx(ctx context.Context, q *platformdb.Queries, id, email, displayName string) (string, error) {
	email = access.NormalizeEmail(email)
	id = strings.TrimSpace(id)
	if id == "" && email != "" {
		id = access.PrincipalIDForEmail(email)
	}
	if id == "" {
		return "", fmt.Errorf("policy principal requires principalId or email")
	}
	if err := q.UpsertPrincipal(ctx, platformdb.UpsertPrincipalParams{
		ID:          id,
		Email:       email,
		DisplayName: firstNonEmpty(strings.TrimSpace(displayName), email, id),
	}); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) ActiveArtifact(ctx context.Context, workspaceID deployment.WorkspaceID, environment deployment.Environment) (deployment.Deployment, deployment.Artifact, error) {
	row, err := r.q.GetActiveDeployment(ctx, platformdb.GetActiveDeploymentParams{WorkspaceID: string(workspaceID), Environment: string(deployment.NormalizeEnvironment(environment))})
	if err != nil {
		return deployment.Deployment{}, deployment.Artifact{}, mapNotFound(err)
	}
	artifact, err := r.q.GetArtifactByDeployment(ctx, row.ID)
	if err != nil {
		return deployment.Deployment{}, deployment.Artifact{}, mapNotFound(err)
	}
	return mapDeployment(row), mapArtifact(artifact), nil
}

func (r *Repository) ArtifactByDeployment(ctx context.Context, deploymentID deployment.ID) (deployment.Artifact, error) {
	artifact, err := r.q.GetArtifactByDeployment(ctx, string(deploymentID))
	if err != nil {
		return deployment.Artifact{}, mapNotFound(err)
	}
	return mapArtifact(artifact), nil
}

func mapDeployment(row platformdb.Deployment) deployment.Deployment {
	out := deployment.Deployment{
		ID:                 deployment.ID(row.ID),
		WorkspaceID:        deployment.WorkspaceID(row.WorkspaceID),
		Environment:        deployment.Environment(row.Environment),
		Status:             deployment.Status(row.Status),
		Source:             deployment.NormalizeSource(deployment.Source(row.Source)),
		Digest:             row.Digest,
		ManifestJSON:       row.ManifestJson,
		CreatedBy:          row.CreatedBy,
		CreatedAt:          row.CreatedAt,
		Error:              row.Error,
		DuckLakeSnapshotID: row.DucklakeSnapshotID,
	}
	if row.ActivatedAt.Valid {
		out.ActivatedAt = row.ActivatedAt.String
	}
	if row.SupersededAt.Valid {
		out.SupersededAt = row.SupersededAt.String
	}
	if row.CleanupAfter.Valid {
		out.CleanupAfter = row.CleanupAfter.String
	}
	return out
}

func mapArtifact(row platformdb.DeploymentArtifact) deployment.Artifact {
	return deployment.Artifact{
		ID:           row.ID,
		DeploymentID: deployment.ID(row.DeploymentID),
		WorkspaceID:  deployment.WorkspaceID(row.WorkspaceID),
		Environment:  deployment.Environment(row.Environment),
		Digest:       row.Digest,
		Format:       row.Format,
		Path:         row.Path,
		DataRoot:     row.DataRoot,
		ManifestJSON: row.ManifestJson,
		SizeBytes:    row.SizeBytes,
		CreatedAt:    row.CreatedAt,
	}
}

func mapArtifactParams(artifact deployment.Artifact) platformdb.InsertDeploymentArtifactParams {
	return platformdb.InsertDeploymentArtifactParams{
		ID:           artifact.ID,
		DeploymentID: string(artifact.DeploymentID),
		WorkspaceID:  string(artifact.WorkspaceID),
		Environment:  string(deployment.NormalizeEnvironment(artifact.Environment)),
		Digest:       artifact.Digest,
		Format:       artifact.Format,
		Path:         artifact.Path,
		DataRoot:     artifact.DataRoot,
		ManifestJson: artifact.ManifestJSON,
		SizeBytes:    artifact.SizeBytes,
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return deployment.ErrNotFound
	}
	return err
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sqliteTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}

func stableAccessID(prefix, workspaceID, name string) string {
	return "cac_" + prefix + "_" + stableID(workspaceID+"|"+name)
}

func sortedWorkspaceGroupNames(values map[string]workspace.WorkspaceGroup) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceRoleBindingNames(values map[string]workspace.WorkspaceRoleBinding) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newID(prefix string) string {
	return prefix + "_" + newSecret()[:24]
}

func newSecret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		return hex.EncodeToString(sum[:])
	}
	return hex.EncodeToString(b[:])
}

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
