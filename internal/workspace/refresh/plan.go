package refresh

import (
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Plan struct {
	TargetType       string
	TargetID         string
	ModelID          string
	Tables           []string
	DependencyTables []string
}

func PlanForPipeline(definition *workspace.Definition, workspaceID, pipelineID string) (Plan, error) {
	if definition == nil {
		return Plan{}, fmt.Errorf("workspace definition is required")
	}
	pipelineID = strings.TrimSpace(pipelineID)
	pipeline, ok := definition.RefreshPipelines[pipelineID]
	if !ok {
		return Plan{}, fmt.Errorf("unknown refresh pipeline %q", pipelineID)
	}
	model, ok := definition.Models[pipeline.SemanticModel]
	if !ok {
		return Plan{}, fmt.Errorf("refresh pipeline %q references unknown semantic model %q", pipelineID, pipeline.SemanticModel)
	}
	order, err := materialize.ModelTableOrder(model)
	if err != nil {
		return Plan{}, err
	}
	targetID := strings.TrimSpace(workspaceID) + "." + pipelineID
	if _, err := localWorkspaceAssetName(workspaceID, targetID); err != nil {
		return Plan{}, err
	}
	return Plan{
		TargetType:       materialize.TargetRefreshPipeline,
		TargetID:         targetID,
		ModelID:          pipeline.SemanticModel,
		Tables:           order,
		DependencyTables: append([]string(nil), order...),
	}, nil
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
