package workspace

import (
	"github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/refreshpipeline"
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
	Dashboards       map[string]*report.Dashboard
	Access           AccessPolicy
	AgentPolicies    map[string]AgentPolicy
	AgentPolicy      AgentPolicy
	RefreshPipelines map[string]refreshpipeline.Definition
	BaseDir          string
	SourceIDs        map[string]string
	SourceFiles      map[string]string
}

type AgentPolicy struct {
	ID           string
	Name         string
	Enabled      bool
	Tools        AgentPolicyTools
	Instructions string
}

type AgentPolicyTools struct {
	Allow []string
	Deny  []string
}
