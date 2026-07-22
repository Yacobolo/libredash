package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	analyticsduckdb "github.com/Yacobolo/leapview/internal/analytics/duckdb"
	dashboardadapter "github.com/Yacobolo/leapview/internal/analytics/duckdb/dashboardadapter"
	analyticsducklake "github.com/Yacobolo/leapview/internal/analytics/ducklake"
	"github.com/Yacobolo/leapview/internal/analytics/resultcache"
	dashboardruntime "github.com/Yacobolo/leapview/internal/dashboard/runtime"
	"github.com/Yacobolo/leapview/internal/dataquery"
	manageddataruntimebinding "github.com/Yacobolo/leapview/internal/manageddata/runtimebinding"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	servingstatefs "github.com/Yacobolo/leapview/internal/servingstate/filesystem"
)

type servingStateRuntimeFactory struct {
	duckDBDir    string
	runtimeDir   string
	environment  *analyticsducklake.Environment
	cachePool    *resultcache.Pool
	resultLimits dataquery.ResultLimits
}

func (f servingStateRuntimeFactory) Prepare(ctx context.Context, input runtimehost.RuntimeInput) (runtimehost.Runtime, error) {
	duckDBDir := runtimeFirstNonEmpty(input.DuckDBDir, f.duckDBDir)
	runtimeDir := runtimeFirstNonEmpty(input.RuntimeDir, f.runtimeDir)
	targetDir := filepath.Join(runtimeDir, string(input.State.ID)+"-"+shortDigest(input.Artifact.Digest))
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := servingstatefs.ExtractArtifact(input.Artifact.Path, targetDir); err != nil {
		return nil, err
	}
	duckDir := filepath.Join(duckDBDir, string(servingstate.NormalizeEnvironment(input.State.Environment)))
	compiled, _, err := servingstatefs.LoadCompiledWorkspaceArtifact(targetDir)
	if err != nil {
		return nil, err
	}
	if compiled.WorkspaceID != string(input.State.WorkspaceID) {
		return nil, fmt.Errorf("compiled artifact workspace = %q, want %q", compiled.WorkspaceID, input.State.WorkspaceID)
	}
	if err := manageddataruntimebinding.BindRoots(compiled.Definition, input.ManagedData); err != nil {
		return nil, err
	}
	factoryOptions := dashboardadapter.Options{
		Database: f.environment, CredentialResolver: analyticsduckdb.EnvironmentCredentialResolver{}, CachePool: f.cachePool, ResultLimits: f.resultLimits, SnapshotID: input.State.DuckLakeSnapshotID,
		ServingStateID: string(input.State.ID), WorkspaceID: string(input.State.WorkspaceID),
		Environment: string(servingstate.NormalizeEnvironment(input.State.Environment)), SemanticModelDigest: input.State.Digest,
		ArtifactDigest: input.Artifact.Digest, SourceDataDigest: input.ManagedData.RevisionID,
	}
	service, err := dashboardruntime.NewFromDefinition(ctx, duckDir, dashboardadapter.NewFactory(factoryOptions), compiled.Definition)
	if err != nil {
		return nil, err
	}
	if input.State.DuckLakeSnapshotID == 0 {
		snapshotID := service.DuckLakeSnapshotID()
		if snapshotID > 0 {
			if err := service.Close(); err != nil {
				return nil, err
			}
			factoryOptions.SnapshotID = snapshotID
			service, err = dashboardruntime.NewFromDefinition(ctx, duckDir, dashboardadapter.NewFactory(factoryOptions), compiled.Definition)
			if err != nil {
				return nil, err
			}
		}
	}
	return service, nil
}

func runtimeFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
