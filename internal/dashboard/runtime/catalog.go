package runtime

import (
	"strings"
	"sync"

	"github.com/Yacobolo/leapview/internal/brand"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/workspace"
	workspacecompiler "github.com/Yacobolo/leapview/internal/workspace/compiler"
)

type CatalogService struct {
	mu        *sync.RWMutex
	workspace *workspace.Definition
	catalog   dashboard.Catalog
}

func NewCatalogService(mu *sync.RWMutex, workspace *workspace.Definition) *CatalogService {
	service := &CatalogService{mu: mu, workspace: workspace}
	service.catalog = service.catalogView()
	return service
}

func (m *Service) Catalog() dashboard.Catalog {
	return m.catalog.Catalog()
}

func (m *Service) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	return m.catalog.WorkspaceAssets(workspaceID, servingStateID)
}

func (s *CatalogService) Catalog() dashboard.Catalog {
	return s.catalog
}

func (s *CatalogService) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.workspace == nil {
		return nil, nil, false
	}
	graph, err := workspacecompiler.ExtractLineage(workspace.WorkspaceID(workspaceID), workspace.ServingStateID(servingStateID), s.workspace)
	if err != nil {
		return nil, nil, false
	}
	return graph.Assets, graph.Edges, true
}

func (s *CatalogService) catalogView() dashboard.Catalog {
	catalog := dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{
			ID:          workspaceID(s.workspace.Catalog.Workspace),
			Title:       workspaceTitle(s.workspace.Catalog.Workspace),
			Description: s.workspace.Catalog.Workspace.Description,
		},
		Models:     make([]dashboard.CatalogModel, 0, len(s.workspace.Catalog.SemanticModels)),
		Dashboards: make([]dashboard.CatalogDashboard, 0, len(s.workspace.Catalog.Dashboards)),
	}
	for _, model := range s.workspace.Catalog.SemanticModels {
		catalog.Models = append(catalog.Models, dashboard.CatalogModel{
			ID:          model.ID,
			Title:       model.Title,
			Description: model.Description,
		})
	}
	for _, report := range s.workspace.Catalog.Dashboards {
		pageCount := 0
		semanticModel := ""
		if loaded, ok := s.workspace.Dashboards[report.ID]; ok {
			pageCount = len(loaded.Pages)
			semanticModel = loaded.SemanticModel
		}
		catalog.Dashboards = append(catalog.Dashboards, dashboard.CatalogDashboard{
			ID:            report.ID,
			Title:         report.Title,
			Description:   report.Description,
			SemanticModel: semanticModel,
			Tags:          append([]string{}, report.Tags...),
			PageCount:     pageCount,
		})
	}
	return catalog
}

func workspaceID(workspace workspace.CatalogWorkspace) string {
	if strings.TrimSpace(workspace.ID) != "" {
		return workspace.ID
	}
	return ""
}

func workspaceTitle(workspace workspace.CatalogWorkspace) string {
	if strings.TrimSpace(workspace.Title) != "" {
		return workspace.Title
	}
	if strings.TrimSpace(workspace.ID) != "" {
		return workspace.ID
	}
	return brand.Name
}
