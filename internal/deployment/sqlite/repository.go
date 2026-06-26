package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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
		Status:      string(deployment.StatusPending),
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

func (r *Repository) List(ctx context.Context, workspaceID deployment.WorkspaceID) ([]deployment.Deployment, error) {
	rows, err := r.q.ListDeployments(ctx, string(workspaceID))
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
	artifact.WorkspaceID = deployment.WorkspaceID(current.WorkspaceID)
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

func (r *Repository) Activate(ctx context.Context, workspaceID deployment.WorkspaceID, deploymentID deployment.ID) (deployment.Deployment, error) {
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
	if !current.CanActivate() {
		return deployment.Deployment{}, fmt.Errorf("deployment %s has status %q, want validated", deploymentID, current.Status)
	}
	if err := q.MarkOtherDeploymentsInactive(ctx, platformdb.MarkOtherDeploymentsInactiveParams{WorkspaceID: string(workspaceID), ID: string(deploymentID)}); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.MarkDeploymentActive(ctx, string(deploymentID)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := q.SetWorkspaceActiveDeployment(ctx, platformdb.SetWorkspaceActiveDeploymentParams{
		ActiveDeploymentID: sql.NullString{String: string(deploymentID), Valid: true},
		ID:                 string(workspaceID),
	}); err != nil {
		return deployment.Deployment{}, err
	}
	if err := tx.Commit(); err != nil {
		return deployment.Deployment{}, err
	}
	return r.ByID(ctx, deploymentID)
}

func (r *Repository) ActiveArtifact(ctx context.Context, workspaceID deployment.WorkspaceID) (deployment.Deployment, deployment.Artifact, error) {
	row, err := r.q.GetActiveDeployment(ctx, string(workspaceID))
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
		ID:           deployment.ID(row.ID),
		WorkspaceID:  deployment.WorkspaceID(row.WorkspaceID),
		Status:       deployment.Status(row.Status),
		Digest:       row.Digest,
		ManifestJSON: row.ManifestJson,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
		Error:        row.Error,
	}
	if row.ActivatedAt.Valid {
		out.ActivatedAt = row.ActivatedAt.String
	}
	return out
}

func mapArtifact(row platformdb.DeploymentArtifact) deployment.Artifact {
	return deployment.Artifact{
		ID:           row.ID,
		DeploymentID: deployment.ID(row.DeploymentID),
		WorkspaceID:  deployment.WorkspaceID(row.WorkspaceID),
		Digest:       row.Digest,
		Format:       row.Format,
		Path:         row.Path,
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
		Digest:       artifact.Digest,
		Format:       artifact.Format,
		Path:         artifact.Path,
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
