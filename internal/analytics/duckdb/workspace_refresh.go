package duckdb

import (
	"context"
	"fmt"

	analyticsducklake "github.com/Yacobolo/leapview/internal/analytics/ducklake"
	manageddataruntimebinding "github.com/Yacobolo/leapview/internal/manageddata/runtimebinding"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
)

type WorkspaceRefreshMaterializer struct {
	Environment *analyticsducklake.Environment
	ManagedData runtimehost.ManagedDataResolver
	Credentials CredentialResolver
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
	runtime, err := OpenWorkspaceMaterializeRuntime(ctx, WorkspaceRuntimeConfig{
		Models:             input.Definition.Models,
		Database:           m.Environment,
		CredentialResolver: m.Credentials,
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
