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
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Repository struct {
	db *sql.DB
	q  *platformdb.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	return &Repository{db: sqlDB, q: platformdb.New(sqlDB)}
}

func (r *Repository) Create(ctx context.Context, input servingstate.CreateInput) (servingstate.State, error) {
	id := servingstate.ID(newID("state"))
	if err := r.q.CreateServingState(ctx, platformdb.CreateServingStateParams{
		ID:          string(id),
		WorkspaceID: string(input.WorkspaceID),
		Environment: string(servingstate.NormalizeEnvironment(input.Environment)),
		Status:      string(servingstate.StatusPending),
		Source:      string(servingstate.NormalizeSource(input.Source)),
		CreatedBy:   input.CreatedBy,
	}); err != nil {
		return servingstate.State{}, err
	}
	return r.ByID(ctx, id)
}

func (r *Repository) ByID(ctx context.Context, id servingstate.ID) (servingstate.State, error) {
	row, err := r.q.GetServingState(ctx, string(id))
	if err != nil {
		return servingstate.State{}, mapNotFound(err)
	}
	return mapServingState(row), nil
}

func (r *Repository) List(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) ([]servingstate.State, error) {
	rows, err := r.q.ListServingStates(ctx, platformdb.ListServingStatesParams{WorkspaceID: string(workspaceID), Environment: string(servingstate.NormalizeEnvironment(environment))})
	if err != nil {
		return nil, err
	}
	states := make([]servingstate.State, 0, len(rows))
	for _, row := range rows {
		states = append(states, mapServingState(row))
	}
	return states, nil
}

func (r *Repository) MarkFailed(ctx context.Context, servingStateID servingstate.ID, cause error) error {
	if cause == nil {
		return nil
	}
	return r.q.UpdateServingStateStatus(ctx, platformdb.UpdateServingStateStatusParams{
		Status: string(servingstate.StatusFailed),
		Error:  cause.Error(),
		ID:     string(servingStateID),
	})
}

func (r *Repository) RecordDuckLakeSnapshot(ctx context.Context, servingStateID servingstate.ID, snapshotID int64) error {
	if snapshotID <= 0 {
		return fmt.Errorf("ducklake snapshot id must be positive")
	}
	return r.q.UpdateServingStateDuckLakeSnapshot(ctx, platformdb.UpdateServingStateDuckLakeSnapshotParams{
		DucklakeSnapshotID: snapshotID,
		ID:                 string(servingStateID),
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

func (r *Repository) CreateQuerySnapshotLease(ctx context.Context, input servingstate.SnapshotLeaseInput) (string, error) {
	if input.WorkspaceID == "" {
		return "", fmt.Errorf("workspace id is required")
	}
	if input.ServingStateID == "" {
		return "", fmt.Errorf("serving state id is required")
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
		Environment:        string(servingstate.NormalizeEnvironment(input.Environment)),
		ServingStateID:     string(input.ServingStateID),
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

func (r *Repository) ExpireInactiveServingStates(ctx context.Context) error {
	return r.q.ExpireInactiveServingStates(ctx)
}

func (r *Repository) ScheduleExpiredServingStateDeletion(ctx context.Context) error {
	return r.q.ScheduleExpiredServingStateDeletion(ctx)
}

func (r *Repository) MarkDeleteScheduledServingStatesDeleted(ctx context.Context) error {
	return r.q.MarkDeleteScheduledServingStatesDeleted(ctx)
}

func (r *Repository) ReconcileRetention(ctx context.Context, now time.Time) error {
	if now.IsZero() {
		now = time.Now()
	}
	if err := r.q.MarkDrainingServingStatesDeleteScheduled(ctx); err != nil {
		return err
	}
	return r.q.MarkDeleteScheduledServingStatesDeleted(ctx)
}

func (r *Repository) SaveValidated(ctx context.Context, servingStateID servingstate.ID, validation servingstate.Validation, artifact servingstate.Artifact) (servingstate.State, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return servingstate.State{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	artifact.ServingStateID = servingStateID
	current, err := q.GetServingState(ctx, string(servingStateID))
	if err != nil {
		return servingstate.State{}, mapNotFound(err)
	}
	if artifact.WorkspaceID != servingstate.WorkspaceID(current.WorkspaceID) {
		return servingstate.State{}, fmt.Errorf("artifact workspace = %q, want %q", artifact.WorkspaceID, current.WorkspaceID)
	}
	if servingstate.NormalizeEnvironment(artifact.Environment) != servingstate.Environment(current.Environment) {
		return servingstate.State{}, fmt.Errorf("artifact environment = %q, want %q", servingstate.NormalizeEnvironment(artifact.Environment), current.Environment)
	}
	if err := workspace.ValidateAssetGraphForServingState(validation.Graph, workspace.WorkspaceID(current.WorkspaceID), workspace.ServingStateID(servingStateID)); err != nil {
		return servingstate.State{}, err
	}
	if err := q.InsertServingStateArtifact(ctx, mapArtifactParams(artifact)); err != nil {
		return servingstate.State{}, err
	}
	if err := q.ClearAssetEdgesForServingState(ctx, string(servingStateID)); err != nil {
		return servingstate.State{}, err
	}
	if err := q.ClearAssetsForServingState(ctx, string(servingStateID)); err != nil {
		return servingstate.State{}, err
	}
	for _, asset := range validation.Graph.Assets {
		if err := q.InsertAsset(ctx, platformdb.InsertAssetParams{
			SnapshotID:           string(asset.SnapshotID),
			LogicalAssetID:       string(asset.ID),
			WorkspaceID:          string(asset.WorkspaceID),
			ServingStateID:       string(asset.ServingStateID),
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
			return servingstate.State{}, err
		}
	}
	for _, edge := range validation.Graph.Edges {
		if err := q.InsertAssetEdge(ctx, platformdb.InsertAssetEdgeParams{
			ID:                 string(edge.ID),
			WorkspaceID:        string(edge.WorkspaceID),
			ServingStateID:     string(edge.ServingStateID),
			FromLogicalAssetID: string(edge.FromAssetID),
			ToLogicalAssetID:   string(edge.ToAssetID),
			EdgeType:           string(edge.Type),
		}); err != nil {
			return servingstate.State{}, err
		}
	}
	if err := q.UpdateServingStateValidated(ctx, platformdb.UpdateServingStateValidatedParams{
		Status:       string(servingstate.StatusValidated),
		Digest:       validation.Digest,
		ManifestJson: validation.ManifestJSON,
		ID:           string(servingStateID),
	}); err != nil {
		return servingstate.State{}, err
	}
	if err := tx.Commit(); err != nil {
		return servingstate.State{}, err
	}
	return r.ByID(ctx, servingStateID)
}

func (r *Repository) Activate(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID) (servingstate.State, error) {
	return r.activate(ctx, workspaceID, environment, servingStateID, nil)
}

func (r *Repository) ActivateWithWorkspacePolicy(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID, policy workspace.AccessPolicy) (servingstate.State, error) {
	return r.activate(ctx, workspaceID, environment, servingStateID, &policy)
}

func (r *Repository) activate(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment, servingStateID servingstate.ID, policy *workspace.AccessPolicy) (servingstate.State, error) {
	environment = servingstate.NormalizeEnvironment(environment)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return servingstate.State{}, err
	}
	defer tx.Rollback()
	q := r.q.WithTx(tx)
	row, err := q.GetServingState(ctx, string(servingStateID))
	if err != nil {
		return servingstate.State{}, mapNotFound(err)
	}
	current := mapServingState(row)
	if current.WorkspaceID != workspaceID {
		return servingstate.State{}, fmt.Errorf("serving state %s is not in workspace %s", servingStateID, workspaceID)
	}
	if current.Environment != environment {
		return servingstate.State{}, fmt.Errorf("serving state %s environment = %q, want %q", servingStateID, current.Environment, environment)
	}
	if !current.CanActivate() {
		return servingstate.State{}, fmt.Errorf("serving state %s has status %q, want validated", servingStateID, current.Status)
	}
	if policy != nil {
		if err := reconcileWorkspacePolicyTx(ctx, q, string(workspaceID), *policy); err != nil {
			return servingstate.State{}, err
		}
	}
	if err := q.MarkOtherServingStatesDraining(ctx, platformdb.MarkOtherServingStatesDrainingParams{
		WorkspaceID: string(workspaceID),
		Environment: string(environment),
		ID:          string(servingStateID),
	}); err != nil {
		return servingstate.State{}, err
	}
	if err := q.MarkServingStateActive(ctx, string(servingStateID)); err != nil {
		return servingstate.State{}, err
	}
	if err := q.SetActiveServingState(ctx, platformdb.SetActiveServingStateParams{
		WorkspaceID:    string(workspaceID),
		Environment:    string(environment),
		ServingStateID: string(servingStateID),
	}); err != nil {
		return servingstate.State{}, err
	}
	if err := tx.Commit(); err != nil {
		return servingstate.State{}, err
	}
	return r.ByID(ctx, servingStateID)
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

func (r *Repository) ActiveArtifact(ctx context.Context, workspaceID servingstate.WorkspaceID, environment servingstate.Environment) (servingstate.State, servingstate.Artifact, error) {
	row, err := r.q.GetActiveServingState(ctx, platformdb.GetActiveServingStateParams{WorkspaceID: string(workspaceID), Environment: string(servingstate.NormalizeEnvironment(environment))})
	if err != nil {
		return servingstate.State{}, servingstate.Artifact{}, mapNotFound(err)
	}
	artifact, err := r.q.GetArtifactByServingState(ctx, row.ID)
	if err != nil {
		return servingstate.State{}, servingstate.Artifact{}, mapNotFound(err)
	}
	return mapServingState(row), mapArtifact(artifact), nil
}

func (r *Repository) ArtifactByServingState(ctx context.Context, servingStateID servingstate.ID) (servingstate.Artifact, error) {
	artifact, err := r.q.GetArtifactByServingState(ctx, string(servingStateID))
	if err != nil {
		return servingstate.Artifact{}, mapNotFound(err)
	}
	return mapArtifact(artifact), nil
}

func mapServingState(row platformdb.ServingState) servingstate.State {
	out := servingstate.State{
		ID:                 servingstate.ID(row.ID),
		WorkspaceID:        servingstate.WorkspaceID(row.WorkspaceID),
		Environment:        servingstate.Environment(row.Environment),
		Status:             servingstate.Status(row.Status),
		Source:             servingstate.NormalizeSource(servingstate.Source(row.Source)),
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
	return out
}

func mapArtifact(row platformdb.ServingStateArtifact) servingstate.Artifact {
	return servingstate.Artifact{
		ID:             row.ID,
		ServingStateID: servingstate.ID(row.ServingStateID),
		WorkspaceID:    servingstate.WorkspaceID(row.WorkspaceID),
		Environment:    servingstate.Environment(row.Environment),
		Digest:         row.Digest,
		Format:         row.Format,
		Path:           row.Path,
		DataRoot:       row.DataRoot,
		ManifestJSON:   row.ManifestJson,
		SizeBytes:      row.SizeBytes,
		CreatedAt:      row.CreatedAt,
	}
}

func mapArtifactParams(artifact servingstate.Artifact) platformdb.InsertServingStateArtifactParams {
	return platformdb.InsertServingStateArtifactParams{
		ID:             artifact.ID,
		ServingStateID: string(artifact.ServingStateID),
		WorkspaceID:    string(artifact.WorkspaceID),
		Environment:    string(servingstate.NormalizeEnvironment(artifact.Environment)),
		Digest:         artifact.Digest,
		Format:         artifact.Format,
		Path:           artifact.Path,
		DataRoot:       artifact.DataRoot,
		ManifestJson:   artifact.ManifestJSON,
		SizeBytes:      artifact.SizeBytes,
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return servingstate.ErrNotFound
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
