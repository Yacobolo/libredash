package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacehttp "github.com/Yacobolo/leapview/internal/workspace/http"
	"github.com/go-chi/chi/v5"
)

// workspaceAssetObjectRefs makes a refresh pipeline inherit the authorization
// boundary of the semantic model it refreshes. Pipelines are configuration and
// are intentionally not independently grantable in v1.
func (s *Server) workspaceAssetObjectRefs(r *http.Request, workspaceID string) []access.ObjectRef {
	rawAssetID := strings.TrimSpace(chi.URLParam(r, "asset"))
	if !strings.HasPrefix(rawAssetID, string(workspace.AssetTypeRefreshPipeline)+":") {
		return workspacehttp.AssetObjectRefs(r, workspaceID)
	}
	assets, edges, err := s.workspaceHTTPReadModel().WorkspaceAssetsAndEdgesForData(r.Context(), workspaceID, string(s.requestServingEnvironment(r)))
	if err != nil {
		return []access.ObjectRef{access.WorkspaceObject(workspaceID)}
	}
	for _, edge := range edges {
		if edge.FromAssetID != rawAssetID || edge.Type != string(workspace.AssetEdgeRefreshesSemanticModel) {
			continue
		}
		for _, asset := range assets {
			if asset.ID != edge.ToAssetID || asset.Type != string(workspace.AssetTypeSemanticModel) {
				continue
			}
			modelID := strings.TrimPrefix(asset.Key, workspaceID+".")
			model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, modelID, access.WorkspaceObject(workspaceID))
			return []access.ObjectRef{model, access.WorkspaceObject(workspaceID)}
		}
	}
	return []access.ObjectRef{access.WorkspaceObject(workspaceID)}
}

func (s *Server) refreshPipelineSemanticModel(ctx context.Context, workspaceID, pipelineID string) (string, bool, error) {
	states, err := s.servingStateRepository()
	if err != nil {
		return "", false, err
	}
	if states == nil {
		return "", false, nil
	}
	_, artifact, err := states.ActiveArtifact(ctx, servingstate.WorkspaceID(workspaceID), s.defaultServingEnvironment())
	if err != nil {
		return "", false, err
	}
	loaded, err := (appRefreshArtifactLoader{}).Load(ctx, artifact)
	if err != nil {
		return "", false, err
	}
	if loaded.Definition == nil {
		return "", false, nil
	}
	pipeline, ok := loaded.Definition.RefreshPipelines[pipelineID]
	if !ok {
		return "", false, nil
	}
	return pipeline.SemanticModel, true, nil
}
