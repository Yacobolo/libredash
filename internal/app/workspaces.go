package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/gorilla/csrf"
)

func (s *Server) assetVersionsStateForSection(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error) {
	state := ui.AssetVersionsState{CurrentContentHash: asset.ContentHash}
	if section != "versions" {
		return state, nil
	}
	if s.store == nil {
		return state, nil
	}
	repo, err := s.workspaceRepository()
	if err != nil || repo == nil {
		return state, err
	}
	versions, err := repo.AssetVersions(ctx, workspace.WorkspaceID(workspaceID), environment, workspace.AssetID(asset.ID))
	if err != nil {
		return state, err
	}
	state.Versions = make([]ui.AssetVersionState, 0, len(versions))
	for _, version := range versions {
		state.Versions = append(state.Versions, ui.AssetVersionState{
			ServingStateID: string(version.ServingStateID),
			Status:         version.Status,
			Digest:         version.Digest,
			CreatedBy:      version.CreatedBy,
			CreatedAt:      version.CreatedAt,
			ActivatedAt:    version.ActivatedAt,
			SourceFile:     version.SourceFile,
			ContentHash:    version.ContentHash,
		})
	}
	return state, nil
}

func (s *Server) workspaceAssetCatalogReader() (workspace.AssetCatalogReader, error) {
	if s.assetCatalog != nil {
		return s.assetCatalog, nil
	}
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	service := workspace.NewAssetCatalogService(repo)
	s.assetCatalog = service
	return s.assetCatalog, nil
}

func csrfToken(r *http.Request, auth *Auth) string {
	if auth == nil {
		return ""
	}
	return csrf.Token(r)
}

func (s *Server) currentRoleLabel(r *http.Request) string {
	if s.auth == nil {
		return "Local"
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return "Signed out"
	}
	if principal.DevBypass {
		return "Platform admin"
	}
	return "Platform access"
}
