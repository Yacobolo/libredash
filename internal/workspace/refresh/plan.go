package refresh

import (
	"fmt"
	"strings"

	analyticsduckdb "github.com/Yacobolo/libredash/internal/analytics/duckdb"
	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/workspace"
)

const WorkspaceRefreshModelID = "workspace"

type Plan struct {
	TargetType       string
	TargetID         string
	ModelID          string
	Tables           []string
	DependencyTables []string
	ChildTrigger     string
}

func PlanForAsset(definition *workspace.Definition, workspaceID string, asset workspace.AssetView) (Plan, error) {
	if definition == nil {
		return Plan{}, fmt.Errorf("workspace definition is required")
	}
	targetID := AssetRefreshTargetID(asset)
	switch asset.Type {
	case string(workspace.AssetTypeSemanticModel):
		modelID, err := localWorkspaceAssetName(workspaceID, asset.Key)
		if err != nil {
			return Plan{}, err
		}
		model, ok := definition.Models[modelID]
		if !ok {
			return Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
		}
		order, err := materialize.ModelTableOrder(model)
		if err != nil {
			return Plan{}, err
		}
		return Plan{
			TargetType:       materialize.TargetSemanticModel,
			TargetID:         targetID,
			ModelID:          modelID,
			Tables:           order,
			DependencyTables: order,
			ChildTrigger:     materialize.TriggerSemanticModel,
		}, nil
	case string(workspace.AssetTypeModelTable):
		tableName, err := localWorkspaceAssetName(workspaceID, asset.Key)
		if err != nil {
			return Plan{}, err
		}
		order, err := analyticsduckdb.WorkspaceModelTableDependencyOrder(definition.Models, tableName)
		if err != nil {
			return Plan{}, err
		}
		dependencies := append([]string(nil), order...)
		if len(dependencies) > 0 && dependencies[len(dependencies)-1] == tableName {
			dependencies = dependencies[:len(dependencies)-1]
		}
		return Plan{
			TargetType:       materialize.TargetModelTable,
			TargetID:         targetID,
			ModelID:          WorkspaceRefreshModelID,
			Tables:           order,
			DependencyTables: dependencies,
			ChildTrigger:     materialize.TriggerDependency,
		}, nil
	default:
		return Plan{}, fmt.Errorf("asset type %q cannot be refreshed", asset.Type)
	}
}

func AssetRefreshTargetID(asset workspace.AssetView) string {
	return asset.Key
}

func AssetTypeForRefreshTarget(targetType string) string {
	switch targetType {
	case materialize.TargetModelTable:
		return string(workspace.AssetTypeModelTable)
	case materialize.TargetSemanticModel:
		return string(workspace.AssetTypeSemanticModel)
	default:
		return targetType
	}
}

func localWorkspaceAssetName(workspaceID, key string) (string, error) {
	prefix := strings.TrimSpace(workspaceID) + "."
	key = strings.TrimSpace(key)
	if prefix == "." {
		return "", fmt.Errorf("workspace id is required")
	}
	if !strings.HasPrefix(key, prefix) {
		return "", fmt.Errorf("asset key %q is not in workspace %q", key, workspaceID)
	}
	name := strings.TrimSpace(strings.TrimPrefix(key, prefix))
	if name == "" {
		return "", fmt.Errorf("asset key %q is missing a local name", key)
	}
	return name, nil
}
