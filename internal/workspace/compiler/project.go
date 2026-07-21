package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/refreshpipeline"
	"github.com/Yacobolo/leapview/internal/workspace"
	"sort"
)

const projectAPIVersion = "leapview.dev/v1"

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
	Publications          map[string]workspace.DashboardPublication
	AccessGroups          map[string]workspace.WorkspaceGroup
	AccessRoleBindings    map[string]workspace.WorkspaceRoleBinding
	AccessGrants          map[string]workspace.WorkspaceGrant
	AccessDataPolicies    map[string]workspace.WorkspaceDataPolicy
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
	PublicationPaths      map[string]string
	AccessPaths           map[string]string
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
		if err := compilePublicationClosures(definition, graph); err != nil {
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

func compilePublicationClosures(definition *workspace.Definition, graph workspace.AssetGraph) error {
	types := make(map[workspace.AssetID]workspace.AssetType, len(graph.Assets))
	parents := make(map[workspace.AssetID]workspace.AssetID, len(graph.Assets))
	for _, asset := range graph.Assets {
		types[asset.ID] = asset.Type
		parents[asset.ID] = asset.ParentID
	}
	adjacent := make(map[workspace.AssetID][]workspace.AssetEdge, len(graph.Assets))
	for _, edge := range graph.Edges {
		adjacent[edge.FromAssetID] = append(adjacent[edge.FromAssetID], edge)
	}
	for name, publication := range definition.Publications {
		root := workspace.NewAssetID(workspace.AssetTypeDashboard, definition.Catalog.Workspace.ID+"."+publication.Dashboard)
		seen := map[workspace.AssetID]struct{}{root: {}}
		queue := []workspace.AssetID{root}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if parent := parents[current]; parent != "" {
				if _, ok := seen[parent]; !ok {
					seen[parent] = struct{}{}
					queue = append(queue, parent)
				}
			}
			for _, edge := range adjacent[current] {
				if edge.Type == workspace.AssetEdgeContains {
					switch types[current] {
					case workspace.AssetTypeCatalog, workspace.AssetTypeSemanticModel, workspace.AssetTypeSemanticTable:
						continue
					}
				}
				next := edge.ToAssetID
				if _, ok := seen[next]; ok {
					continue
				}
				seen[next] = struct{}{}
				queue = append(queue, next)
			}
		}
		publication.DependencyAssetIDs = make([]string, 0, len(seen))
		for id := range seen {
			publication.DependencyAssetIDs = append(publication.DependencyAssetIDs, string(id))
		}
		sort.Strings(publication.DependencyAssetIDs)
		publication.ConfigurationDigest = ""
		payload, err := json.Marshal(publication)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(payload)
		publication.ConfigurationDigest = "sha256:" + hex.EncodeToString(sum[:])
		definition.Publications[name] = publication
	}
	return nil
}
