package duckdb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

type WorkspaceRefreshMaterializer struct {
	DuckDBDir       string
	DuckLakeCatalog string
	DuckLakeData    string
}

func (m WorkspaceRefreshMaterializer) Materialize(ctx context.Context, input refresh.MaterializeInput) (int64, error) {
	dbDir := m.DuckDBDir
	if strings.TrimSpace(dbDir) == "" {
		dbDir = filepath.Join(".libredash", "duckdb")
	}
	dbDir = filepath.Join(dbDir, string(servingstate.NormalizeEnvironment(input.Environment)))
	runtime, err := OpenWorkspaceMaterializeRuntime(ctx, WorkspaceRuntimeConfig{
		Models:             input.Definition.Models,
		DBDir:              dbDir,
		CatalogPath:        m.DuckLakeCatalog,
		DuckLakeDataPath:   m.DuckLakeData,
		ServingStateID:     string(input.Candidate.ID),
		WorkspaceID:        string(input.Candidate.WorkspaceID),
		Environment:        string(servingstate.NormalizeEnvironment(input.Environment)),
		TargetType:         input.Plan.TargetType,
		TargetID:           input.Plan.TargetID,
		SemanticDigest:     input.Candidate.Digest,
		ArtifactDigest:     input.Artifact.Digest,
		SkipInitialRefresh: true,
	})
	if err != nil {
		return 0, err
	}
	defer runtime.Close()
	if err := runtime.RefreshWorkspaceTables(ctx, input.Plan.Tables); err != nil {
		return 0, err
	}
	snapshotID := runtime.DuckLakeSnapshotID()
	if snapshotID <= 0 {
		return 0, fmt.Errorf("refresh did not produce a DuckLake snapshot")
	}
	return snapshotID, nil
}
