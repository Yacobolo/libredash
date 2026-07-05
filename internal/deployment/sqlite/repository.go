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
	assets, err := q.ListAssetsByDeployment(ctx, string(deploymentID))
	if err != nil {
		return deployment.Deployment{}, err
	}
	if err := registerDeploymentSecurablesTx(ctx, tx, string(workspaceID), assets); err != nil {
		return deployment.Deployment{}, err
	}
	if policy != nil {
		if err := reconcileWorkspacePolicyTx(ctx, tx, q, string(workspaceID), *policy); err != nil {
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

func reconcileWorkspacePolicyTx(ctx context.Context, tx *sql.Tx, q *platformdb.Queries, workspaceID string, policy workspace.AccessPolicy) error {
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
	if _, err := tx.ExecContext(ctx, `
DELETE FROM grants
WHERE object_id IN (
  SELECT id FROM securable_objects WHERE workspace_id = ? OR id = ?
)
`, workspaceID, access.WorkspaceObject(workspaceID).CanonicalID()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM data_policies WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
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
		privileges, err := rolePrivilegesTx(ctx, tx, binding.Role)
		if err != nil {
			return err
		}
		subjectType, subjectID, err := roleBindingSubject(params)
		if err != nil {
			return err
		}
		for _, privilege := range privileges {
			if err := upsertGrantTx(ctx, tx, "grant_"+params.ID+"_"+strings.ToLower(privilege), access.WorkspaceObject(workspaceID), subjectType, subjectID, privilege); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedWorkspaceGrantNames(policy.Grants) {
		grant := policy.Grants[name]
		subjectType, subjectID, err := policySubjectTx(ctx, q, workspaceID, grant.Subject, groupIDs)
		if err != nil {
			return fmt.Errorf("workspace grant %q: %w", name, err)
		}
		if err := upsertGrantTx(ctx, tx, stableAccessID("grant", workspaceID, name), policyObjectRef(workspaceID, grant.Object), subjectType, subjectID, grant.Privilege); err != nil {
			return err
		}
	}
	for _, name := range sortedWorkspaceDataPolicyNames(policy.DataPolicies) {
		dataPolicy := policy.DataPolicies[name]
		objectID, err := ensureSecurableObjectTx(ctx, tx, policyObjectRef(workspaceID, dataPolicy.Object))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO data_policies (id, workspace_id, object_id, policy_type, expression_json)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  workspace_id = excluded.workspace_id,
  object_id = excluded.object_id,
  policy_type = excluded.policy_type,
  expression_json = excluded.expression_json,
  updated_at = CURRENT_TIMESTAMP
`, stableAccessID("datapolicy", workspaceID, name), workspaceID, objectID, dataPolicy.PolicyType, dataPolicy.ExpressionJSON); err != nil {
			return err
		}
	}
	return nil
}

func roleBindingSubject(params platformdb.InsertRoleBindingParams) (access.SubjectType, string, error) {
	if params.GroupID.Valid {
		return access.SubjectGroup, params.GroupID.String, nil
	}
	if params.PrincipalID.Valid {
		return access.SubjectPrincipal, params.PrincipalID.String, nil
	}
	return "", "", fmt.Errorf("role binding subject is required")
}

func rolePrivilegesTx(ctx context.Context, tx *sql.Tx, roleName string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT privilege FROM role_grant_templates WHERE role_name = ? ORDER BY privilege`, roleName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	privileges := []string{}
	for rows.Next() {
		var privilege string
		if err := rows.Scan(&privilege); err != nil {
			return nil, err
		}
		privileges = append(privileges, privilege)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(privileges) == 0 {
		return nil, fmt.Errorf("role %q has no grant template", roleName)
	}
	return privileges, nil
}

func policySubjectTx(ctx context.Context, q *platformdb.Queries, workspaceID string, subject workspace.WorkspaceRoleBindingSubject, groupIDs map[string]string) (access.SubjectType, string, error) {
	switch subject.Kind {
	case string(access.SubjectGroup):
		groupID := groupIDs[subject.Group]
		if groupID == "" {
			return "", "", fmt.Errorf("unknown group %q", subject.Group)
		}
		return access.SubjectGroup, groupID, nil
	case string(access.SubjectPrincipal):
		principalID, err := upsertPolicyPrincipalTx(ctx, q, subject.PrincipalID, subject.Email, subject.DisplayName)
		if err != nil {
			return "", "", err
		}
		return access.SubjectPrincipal, principalID, nil
	case string(access.SubjectServicePrincipal):
		id := strings.TrimSpace(subject.PrincipalID)
		if id == "" {
			return "", "", fmt.Errorf("service principal subject requires principalId")
		}
		if err := q.UpsertPrincipal(ctx, platformdb.UpsertPrincipalParams{
			ID:          id,
			Kind:        string(access.PrincipalKindServicePrincipal),
			DisplayName: firstNonEmpty(strings.TrimSpace(subject.DisplayName), id),
		}); err != nil {
			return "", "", err
		}
		return access.SubjectServicePrincipal, id, nil
	default:
		return "", "", fmt.Errorf("unsupported subject kind %q in workspace %q", subject.Kind, workspaceID)
	}
}

func policyObjectRef(workspaceID string, object workspace.WorkspaceSecurableObjectRef) access.ObjectRef {
	typ := access.SecurableType(strings.TrimSpace(object.Type))
	objectID := strings.TrimSpace(object.ID)
	switch typ {
	case access.SecurableWorkspace:
		return access.WorkspaceObject(workspaceID)
	case access.SecurableDataset, access.SecurableTable:
		if modelID, _, ok := strings.Cut(objectID, "/"); ok && strings.TrimSpace(modelID) != "" {
			return access.ItemObjectWithParent(typ, workspaceID, objectID, access.ItemObject(access.SecurableSemanticModel, workspaceID, modelID))
		}
	case access.SecurableColumn:
		parts := strings.Split(objectID, "/")
		if len(parts) >= 3 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			parent := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, parts[0]+"/"+parts[1], access.ItemObject(access.SecurableSemanticModel, workspaceID, parts[0]))
			return access.ItemObjectWithParent(typ, workspaceID, objectID, parent)
		}
	}
	return access.ItemObject(typ, workspaceID, objectID)
}

func registerDeploymentSecurablesTx(ctx context.Context, tx *sql.Tx, workspaceID string, assets []platformdb.Asset) error {
	workspaceObject := access.WorkspaceObject(workspaceID)
	if _, err := ensureSecurableObjectTx(ctx, tx, workspaceObject); err != nil {
		return err
	}
	for _, asset := range assets {
		parents, object, ok := securableRefsForAsset(workspaceID, asset)
		if !ok {
			continue
		}
		for _, parent := range parents {
			if _, err := ensureSecurableObjectTx(ctx, tx, parent); err != nil {
				return err
			}
		}
		if _, err := ensureSecurableObjectTx(ctx, tx, object); err != nil {
			return err
		}
	}
	return nil
}

func securableRefsForAsset(workspaceID string, asset platformdb.Asset) ([]access.ObjectRef, access.ObjectRef, bool) {
	key := runtimeAssetKey(workspaceID, asset.AssetKey)
	workspaceObject := access.WorkspaceObject(workspaceID)
	switch workspace.AssetType(asset.AssetType) {
	case workspace.AssetTypeDashboard:
		return []access.ObjectRef{workspaceObject}, access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, key, workspaceObject), key != ""
	case workspace.AssetTypeSemanticModel:
		return []access.ObjectRef{workspaceObject}, access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, key, workspaceObject), key != ""
	case workspace.AssetTypeSource:
		return []access.ObjectRef{workspaceObject}, access.ItemObjectWithParent(access.SecurableSource, workspaceID, key, workspaceObject), key != ""
	case workspace.AssetTypeWorkspaceAgentPolicy:
		return []access.ObjectRef{workspaceObject}, access.ItemObjectWithParent(access.SecurableAgentPolicy, workspaceID, key, workspaceObject), key != ""
	case workspace.AssetTypeModelTable:
		return []access.ObjectRef{workspaceObject}, access.ItemObjectWithParent(access.SecurableModelTable, workspaceID, key, workspaceObject), key != ""
	case workspace.AssetTypeSemanticTable:
		modelID, tableID, ok := splitModelTableKey(key)
		if !ok {
			return nil, access.ObjectRef{}, false
		}
		model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, workspaceObject)
		table := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+tableID, model)
		return []access.ObjectRef{workspaceObject, model}, table, true
	case workspace.AssetTypeField:
		modelID, tableID, columnID, ok := splitModelTableColumnKey(key)
		if !ok {
			return nil, access.ObjectRef{}, false
		}
		model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, workspaceObject)
		table := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, modelID+"/"+tableID, model)
		column := access.ItemObjectWithParent(access.SecurableColumn, workspaceID, modelID+"/"+tableID+"/"+columnID, table)
		return []access.ObjectRef{workspaceObject, model, table}, column, true
	default:
		return nil, access.ObjectRef{}, false
	}
}

func runtimeAssetKey(workspaceID, key string) string {
	key = strings.TrimSpace(key)
	return strings.TrimPrefix(key, strings.TrimSpace(workspaceID)+".")
}

func splitModelTableKey(key string) (string, string, bool) {
	modelID, tableID, ok := strings.Cut(strings.TrimSpace(key), ".")
	return modelID, tableID, ok && modelID != "" && tableID != ""
}

func splitModelTableColumnKey(key string) (string, string, string, bool) {
	modelID, rest, ok := strings.Cut(strings.TrimSpace(key), ".")
	if !ok || modelID == "" {
		return "", "", "", false
	}
	tableID, columnID, ok := strings.Cut(rest, ".")
	return modelID, tableID, columnID, ok && tableID != "" && columnID != ""
}

func upsertGrantTx(ctx context.Context, tx *sql.Tx, id string, object access.ObjectRef, subjectType access.SubjectType, subjectID, privilege string) error {
	if strings.TrimSpace(subjectID) == "" {
		return fmt.Errorf("grant subject id is required")
	}
	if subjectType == "" {
		return fmt.Errorf("grant subject type is required")
	}
	if strings.TrimSpace(privilege) == "" {
		return fmt.Errorf("grant privilege is required")
	}
	objectID, err := ensureSecurableObjectTx(ctx, tx, object)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO grants (id, object_id, subject_type, subject_id, privilege)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(object_id, subject_type, subject_id, privilege) DO UPDATE SET id = excluded.id
`, id, objectID, string(subjectType), strings.TrimSpace(subjectID), privilege)
	return err
}

func ensureSecurableObjectTx(ctx context.Context, tx *sql.Tx, object access.ObjectRef) (string, error) {
	objectID := object.CanonicalID()
	parentID := ""
	if strings.TrimSpace(object.ParentID) != "" {
		parentID = strings.TrimSpace(object.ParentID)
	} else if parent, ok := object.Parent(); ok {
		parentID = parent.CanonicalID()
		if _, err := ensureSecurableObjectTx(ctx, tx, parent); err != nil {
			return "", err
		}
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO securable_objects (id, object_type, workspace_id, parent_id, display_name)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  object_type = excluded.object_type,
  workspace_id = excluded.workspace_id,
  parent_id = excluded.parent_id,
  display_name = COALESCE(NULLIF(excluded.display_name, ''), securable_objects.display_name),
  updated_at = CURRENT_TIMESTAMP
`, objectID, string(object.Type), object.WorkspaceID, parentID, securableDisplayName(object))
	return objectID, err
}

func securableDisplayName(object access.ObjectRef) string {
	if object.ObjectID != "" {
		return object.ObjectID
	}
	if object.WorkspaceID != "" {
		return object.WorkspaceID
	}
	return string(object.Type)
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
		Kind:        string(access.PrincipalKindUser),
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

func sortedWorkspaceGrantNames(values map[string]workspace.WorkspaceGrant) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedWorkspaceDataPolicyNames(values map[string]workspace.WorkspaceDataPolicy) []string {
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
