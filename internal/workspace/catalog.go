package workspace

import (
	"github.com/Yacobolo/leapview/internal/analytics/model"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
)

type Catalog struct {
	Workspace      CatalogWorkspace   `yaml:"workspace"`
	SemanticModels []CatalogModel     `yaml:"semantic_models"`
	Dashboards     []CatalogDashboard `yaml:"dashboards"`
}

type CatalogWorkspace struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

type CatalogModel struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type CatalogDashboard struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Path        string   `yaml:"path"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
}

type Definition struct {
	Catalog          Catalog
	Models           map[string]*model.Model
	Dashboards       map[string]dashboarddefinition.Definition
	Publications     map[string]DashboardPublication
	Access           AccessPolicy
	RefreshPipelines map[string]refreshpipeline.Definition
	BaseDir          string
	SourceIDs        map[string]string
	SourceFiles      map[string]string
}
