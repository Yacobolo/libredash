package platform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const (
	DefaultWorkspaceID = "libredash"

	PermissionDashboardView           = "dashboard:view"
	PermissionDeploymentCreate        = "deployment:create"
	PermissionDeploymentActivate      = "deployment:activate"
	PermissionDeploymentRollback      = "deployment:rollback"
	PermissionMaterializationsRefresh = "materializations:refresh"
	PermissionRBACManage              = "rbac:manage"
)

var rolePermissions = map[string][]string{
	"owner": {
		PermissionDashboardView,
		PermissionDeploymentCreate,
		PermissionDeploymentActivate,
		PermissionDeploymentRollback,
		PermissionMaterializationsRefresh,
		PermissionRBACManage,
	},
	"admin": {
		PermissionDashboardView,
		PermissionDeploymentCreate,
		PermissionDeploymentActivate,
		PermissionDeploymentRollback,
		PermissionMaterializationsRefresh,
		PermissionRBACManage,
	},
	"deployer": {
		PermissionDashboardView,
		PermissionDeploymentCreate,
		PermissionDeploymentActivate,
		PermissionDeploymentRollback,
		PermissionMaterializationsRefresh,
	},
	"editor": {
		PermissionDashboardView,
		PermissionMaterializationsRefresh,
	},
	"viewer": {
		PermissionDashboardView,
	},
}

type Paths struct {
	HomeDir     string
	DBPath      string
	ArtifactDir string
	DuckDBDir   string
}

func DefaultPaths() Paths {
	home := os.Getenv("LIBREDASH_HOME")
	if home == "" {
		home = ".libredash"
	}
	return Paths{
		HomeDir:     home,
		DBPath:      filepath.Join(home, "libredash.db"),
		ArtifactDir: filepath.Join(home, "artifacts"),
		DuckDBDir:   filepath.Join(home, "duckdb"),
	}
}

type Store struct {
	db *sql.DB
	q  *db.Queries
}

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	store := &Store{db: conn, q: db.New(conn)}
	if err := store.migrate(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if err := store.SeedDefaults(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Queries() *db.Queries {
	return s.q
}

func (s *Store) migrate(ctx context.Context) error {
	for _, stmt := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, s.db, "migrations"); err != nil {
		return fmt.Errorf("migrating platform db: %w", err)
	}
	return nil
}

func (s *Store) SeedDefaults(ctx context.Context) error {
	for name, permissions := range rolePermissions {
		bytes, err := json.Marshal(permissions)
		if err != nil {
			return err
		}
		if err := s.q.UpsertRole(ctx, db.UpsertRoleParams{
			ID:              "role_" + name,
			Name:            name,
			PermissionsJson: string(bytes),
		}); err != nil {
			return err
		}
	}
	return nil
}

type WorkspaceInput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (s *Store) EnsureWorkspace(ctx context.Context, input WorkspaceInput) error {
	if strings.TrimSpace(input.ID) == "" {
		input.ID = DefaultWorkspaceID
	}
	if strings.TrimSpace(input.Title) == "" {
		input.Title = input.ID
	}
	return s.q.UpsertWorkspace(ctx, db.UpsertWorkspaceParams{
		ID:          input.ID,
		Title:       input.Title,
		Description: input.Description,
	})
}

func (s *Store) CreateDeployment(ctx context.Context, workspaceID, createdBy string) (db.Deployment, error) {
	id := newID("dep")
	if err := s.q.CreateDeployment(ctx, db.CreateDeploymentParams{
		ID:          id,
		WorkspaceID: workspaceID,
		Status:      "pending",
		CreatedBy:   createdBy,
	}); err != nil {
		return db.Deployment{}, err
	}
	return s.q.GetDeployment(ctx, id)
}

func (s *Store) ValidateDeployment(ctx context.Context, deploymentID, digest, manifestJSON string, artifact db.InsertDeploymentArtifactParams, assets []Asset, edges []AssetEdge) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	if err := q.InsertDeploymentArtifact(ctx, artifact); err != nil {
		return err
	}
	if err := q.ClearAssetsForDeployment(ctx, deploymentID); err != nil {
		return err
	}
	for _, asset := range assets {
		if err := q.InsertAsset(ctx, db.InsertAssetParams{
			ID:            asset.ID,
			WorkspaceID:   asset.WorkspaceID,
			DeploymentID:  asset.DeploymentID,
			AssetType:     asset.Type,
			AssetKey:      asset.Key,
			ParentAssetID: sql.NullString{String: asset.ParentID, Valid: asset.ParentID != ""},
			Title:         asset.Title,
			Description:   asset.Description,
			ContentJson:   asset.ContentJSON,
			ContentHash:   asset.ContentHash,
		}); err != nil {
			return err
		}
	}
	for _, edge := range edges {
		if err := q.InsertAssetEdge(ctx, db.InsertAssetEdgeParams{
			ID:           edge.ID,
			WorkspaceID:  edge.WorkspaceID,
			DeploymentID: edge.DeploymentID,
			FromAssetID:  edge.FromAssetID,
			ToAssetID:    edge.ToAssetID,
			EdgeType:     edge.Type,
		}); err != nil {
			return err
		}
	}
	if err := q.UpdateDeploymentValidated(ctx, db.UpdateDeploymentValidatedParams{
		Status:       "validated",
		Digest:       digest,
		ManifestJson: manifestJSON,
		ID:           deploymentID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ActivateDeployment(ctx context.Context, workspaceID, deploymentID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	deployment, err := q.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	if deployment.WorkspaceID != workspaceID {
		return fmt.Errorf("deployment %s is not in workspace %s", deploymentID, workspaceID)
	}
	if deployment.Status != "validated" && deployment.Status != "inactive" && deployment.Status != "active" {
		return fmt.Errorf("deployment %s has status %q, want validated", deploymentID, deployment.Status)
	}
	if err := q.MarkOtherDeploymentsInactive(ctx, db.MarkOtherDeploymentsInactiveParams{WorkspaceID: workspaceID, ID: deploymentID}); err != nil {
		return err
	}
	if err := q.MarkDeploymentActive(ctx, deploymentID); err != nil {
		return err
	}
	if err := q.SetWorkspaceActiveDeployment(ctx, db.SetWorkspaceActiveDeploymentParams{
		ActiveDeploymentID: sql.NullString{String: deploymentID, Valid: true},
		ID:                 workspaceID,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkDeploymentFailed(ctx context.Context, deploymentID string, err error) error {
	if err == nil {
		return nil
	}
	return s.q.UpdateDeploymentStatus(ctx, db.UpdateDeploymentStatusParams{
		Status: "failed",
		Error:  err.Error(),
		ID:     deploymentID,
	})
}

func (s *Store) ActiveArtifact(ctx context.Context, workspaceID string) (db.Deployment, db.DeploymentArtifact, error) {
	deployment, err := s.q.GetActiveDeployment(ctx, workspaceID)
	if err != nil {
		return db.Deployment{}, db.DeploymentArtifact{}, err
	}
	artifact, err := s.q.GetArtifactByDeployment(ctx, deployment.ID)
	if err != nil {
		return db.Deployment{}, db.DeploymentArtifact{}, err
	}
	return deployment, artifact, nil
}

func IgnoreNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
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

func stableID(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(value)))
	return hex.EncodeToString(sum[:])[:32]
}

func PrincipalIDForEmail(email string) string {
	return "email_" + stableID(normalizeEmail(email))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
