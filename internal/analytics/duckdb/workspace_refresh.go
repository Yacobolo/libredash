package duckdb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	manageddataruntimebinding "github.com/Yacobolo/leapview/internal/manageddata/runtimebinding"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
)

type WorkspaceRefreshMaterializer struct {
	DuckDBDir       string
	DuckLakeCatalog string
	DuckLakeData    string
	ManagedData     runtimehost.ManagedDataResolver
}

func (m WorkspaceRefreshMaterializer) Materialize(ctx context.Context, input refresh.MaterializeInput) (snapshotID int64, err error) {
	if m.ManagedData != nil {
		resolution, resolveErr := m.ManagedData.ResolveManagedData(ctx, input.Candidate.ID)
		if resolveErr != nil {
			return 0, resolveErr
		}
		if resolution.Lifetime != nil {
			defer func() {
				if releaseErr := resolution.Lifetime.Release(); err == nil && releaseErr != nil {
					snapshotID = 0
					err = fmt.Errorf("release managed data after workspace refresh: %w", releaseErr)
				}
			}()
		}
		if bindErr := manageddataruntimebinding.BindRoots(input.Definition, resolution); bindErr != nil {
			return 0, bindErr
		}
	}
	dbDir := m.DuckDBDir
	if strings.TrimSpace(dbDir) == "" {
		dbDir = filepath.Join(".leapview", "duckdb")
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
	snapshotID = runtime.DuckLakeSnapshotID()
	if snapshotID <= 0 {
		return 0, fmt.Errorf("refresh did not produce a DuckLake snapshot")
	}
	return snapshotID, nil
}
