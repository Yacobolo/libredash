package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	dashboardadapter "github.com/Yacobolo/libredash/internal/analytics/duckdb/dashboardadapter"
	dashboardruntime "github.com/Yacobolo/libredash/internal/dashboard/runtime"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	servingstatefs "github.com/Yacobolo/libredash/internal/servingstate/filesystem"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type servingStateRuntimeFactory struct {
	duckDBDir        string
	runtimeDir       string
	catalogPath      string
	duckLakeDataPath string
}

func (f servingStateRuntimeFactory) Prepare(_ context.Context, input runtimehost.RuntimeInput) (runtimehost.Runtime, error) {
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
	if err := bindManagedDataRoots(compiled.Definition, input.ManagedData); err != nil {
		return nil, err
	}
	dataPath := runtimeFirstNonEmpty(f.duckLakeDataPath, filepath.Join(duckDir, "data"))
	factoryOptions := dashboardadapter.Options{
		SnapshotID: input.State.DuckLakeSnapshotID, CatalogPath: f.catalogPath, DuckLakeDataPath: dataPath,
		ServingStateID: string(input.State.ID), WorkspaceID: string(input.State.WorkspaceID),
		Environment: string(servingstate.NormalizeEnvironment(input.State.Environment)), SemanticModelDigest: input.State.Digest,
		ArtifactDigest: input.Artifact.Digest, SourceDataDigest: input.ManagedData.RevisionID,
	}
	service, err := dashboardruntime.NewFromDefinition(duckDir, dashboardadapter.NewFactory(factoryOptions), compiled.Definition)
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
			service, err = dashboardruntime.NewFromDefinition(duckDir, dashboardadapter.NewFactory(factoryOptions), compiled.Definition)
			if err != nil {
				return nil, err
			}
		}
	}
	return service, nil
}

func bindManagedDataRoots(definition *workspace.Definition, resolution runtimehost.ManagedDataResolution) error {
	if definition == nil {
		return fmt.Errorf("workspace definition is required")
	}
	for modelID, model := range definition.Models {
		if model == nil {
			continue
		}
		for connectionName, connection := range model.Connections {
			if connection.Kind != "managed" {
				continue
			}
			root := filepath.Clean(resolution.Roots[connectionName])
			if resolution.Roots[connectionName] == "" {
				return fmt.Errorf("semantic model %q managed connection %q has no bound revision", modelID, connectionName)
			}
			if !filepath.IsAbs(root) {
				return fmt.Errorf("semantic model %q managed connection %q revision root must be absolute", modelID, connectionName)
			}
			connection.Root = root
			connection.Scope = ""
			model.Connections[connectionName] = connection
		}
	}
	return nil
}

func runtimeFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
