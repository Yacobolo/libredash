package app

import (
	"net/http"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/ui"
	workspacehttp "github.com/Yacobolo/leapview/internal/workspace/http"
)

func (s *Server) workspaceHTTPHandler() workspacehttp.Handler {
	return workspacehttp.Handler{
		WorkspaceID:      s.workspaceID,
		Environment:      func(r *http.Request) string { return string(s.requestServingEnvironment(r)) },
		ReadModel:        s.workspaceHTTPReadModel(),
		RefreshState:     s.workspaceRefreshSupport(),
		RefreshRunner:    s.workspaceRefreshSupport(),
		Broker:           s.broker,
		CSRFToken:        func(r *http.Request) string { return csrfToken(r, s.auth) },
		CurrentRoleLabel: s.currentRoleLabel,
		ChromeOptions:    func(r *http.Request) []ui.ChromeOption { return []ui.ChromeOption{s.chatChromeOption(r)} },
	}
}

func (s *Server) workspaceHTTPReadModel() workspacehttp.ReadModel {
	return workspacehttp.ReadModel{
		WorkspaceRepository: s.workspaceRepository,
		AccessRepository:    s.accessRepository,
		AssetCatalogReader: func() (workspacehttp.AssetCatalogReader, error) {
			return s.workspaceAssetCatalogReader()
		},
		MetricsForWorkspace: s.workspaceHTTPMetrics,
		CatalogForWorkspace: s.catalogForWorkspace,
		RootCatalog: func() dashboard.Catalog {
			if s.metrics == nil {
				return dashboard.Catalog{}
			}
			return s.metrics.Catalog()
		},
		Environment:      func(r *http.Request) string { return string(s.requestServingEnvironment(r)) },
		CurrentPrincipal: s.currentWorkspacePrincipal,
		AuthConfigured:   s.auth != nil,
	}
}

func (s *Server) workspaceHTTPMetrics(workspaceID string) (workspacehttp.Metrics, bool) {
	metrics, ok := s.metricsForWorkspace(workspaceID)
	if !ok {
		return nil, false
	}
	return metrics, true
}

func (s *Server) currentWorkspacePrincipal(r *http.Request) (workspacehttp.Principal, bool) {
	if s.auth == nil {
		principal := localDeveloperPrincipal()
		return workspacehttp.Principal{
			ID:          principal.ID,
			Email:       principal.Email,
			DisplayName: principal.DisplayName,
			DevBypass:   principal.DevBypass,
		}, true
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return workspacehttp.Principal{}, false
	}
	return workspacehttp.Principal{
		ID:          principal.ID,
		Email:       principal.Email,
		DisplayName: principal.DisplayName,
		DevBypass:   principal.DevBypass,
	}, true
}
