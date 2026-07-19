package compiler

import (
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/refreshpipeline"
	"github.com/Yacobolo/libredash/internal/workspace"
)

const projectAPIVersion = "libredash.dev/v1"

type Project struct {
	Name            string
	BaseDir         string
	Connections     map[string]semanticmodel.Connection
	ConnectionPaths map[string]string
	Sources         map[string]semanticmodel.Source
	SourcePaths     map[string]string
	Workspaces      map[string]*WorkspaceProject
}

type WorkspaceProject struct {
	ID                    string
	Title                 string
	Description           string
	AllowedSources        map[string]struct{}
	Models                map[string]semanticmodel.Table
	SemanticModels        map[string]projectSemanticModelSpec
	Dashboards            map[string]*report.Dashboard
	AccessGroups          map[string]workspace.WorkspaceGroup
	AccessRoleBindings    map[string]workspace.WorkspaceRoleBinding
	AccessGrants          map[string]workspace.WorkspaceGrant
	AccessDataPolicies    map[string]workspace.WorkspaceDataPolicy
	AgentPolicies         map[string]workspace.AgentPolicy
	RefreshPipelines      map[string]refreshpipeline.Definition
	ModelTitles           map[string]string
	ModelDescriptions     map[string]string
	DashboardTitles       map[string]string
	DashboardDescriptions map[string]string
	DashboardTags         map[string][]string
	Path                  string
	ModelPaths            map[string]string
	SemanticModelPaths    map[string]string
	DashboardPaths        map[string]string
	AccessPaths           map[string]string
	AgentPolicyPaths      map[string]string
	RefreshPipelinePaths  map[string]string
}

type CompiledProject struct {
	Project    Project
	Workspaces map[string]CompiledWorkspace
}

func CompileProject(projectPath string, opts Options) (CompiledProject, error) {
	project, err := LoadProject(projectPath)
	if err != nil {
		return CompiledProject{}, err
	}
	out := CompiledProject{Project: project, Workspaces: map[string]CompiledWorkspace{}}
	for id, workspaceProject := range project.Workspaces {
		definition, err := workspaceProject.definition(project)
		if err != nil {
			return CompiledProject{}, err
		}
		servingStateID := opts.ServingStateID
		workspaceID := workspace.WorkspaceID(id)
		graph, err := ExtractLineage(workspaceID, servingStateID, definition)
		if err != nil {
			return CompiledProject{}, err
		}
		out.Workspaces[id] = CompiledWorkspace{
			Workspace: workspace.Workspace{
				ID:          workspaceID,
				Title:       workspaceProject.Title,
				Description: workspaceProject.Description,
				BaseDir:     project.BaseDir,
				Graph:       graph,
			},
			Definition: definition,
		}
	}
	return out, nil
}
