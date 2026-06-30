package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Yacobolo/libredash/internal/access"
	analyticsmaterialize "github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/configschema"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace"
	"gopkg.in/yaml.v3"
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
}

type CompiledProject struct {
	Project    Project
	Workspaces map[string]CompiledWorkspace
}

type ProjectPlan struct {
	Project    string                 `json:"project"`
	Workspaces []ProjectPlanWorkspace `json:"workspaces"`
}

type ProjectPlanWorkspace struct {
	ID                string                        `json:"id"`
	Connections       []string                      `json:"connections"`
	Sources           []string                      `json:"sources"`
	ModelTables       []string                      `json:"modelTables"`
	SemanticModels    []string                      `json:"semanticModels"`
	Dashboards        []string                      `json:"dashboards"`
	Changes           []ProjectPlanChange           `json:"changes,omitempty"`
	DependencyChanges []ProjectPlanDependencyChange `json:"dependencyChanges,omitempty"`
	Summary           ProjectPlanSummary            `json:"summary,omitempty"`
}

type ProjectPlanSummary struct {
	Added                 int  `json:"added,omitempty"`
	Changed               int  `json:"changed,omitempty"`
	Removed               int  `json:"removed,omitempty"`
	DependencyChanges     int  `json:"dependencyChanges,omitempty"`
	Breaking              bool `json:"breaking,omitempty"`
	MaterializationImpact bool `json:"materializationImpact,omitempty"`
	AccessImpact          bool `json:"accessImpact,omitempty"`
}

type ProjectPlanChange struct {
	Action                string `json:"action"`
	ID                    string `json:"id"`
	Type                  string `json:"type"`
	Key                   string `json:"key"`
	Reason                string `json:"reason,omitempty"`
	Breaking              bool   `json:"breaking,omitempty"`
	MaterializationImpact bool   `json:"materializationImpact,omitempty"`
	AccessImpact          bool   `json:"accessImpact,omitempty"`
}

type ProjectPlanDependencyChange struct {
	Action string `json:"action"`
	From   string `json:"from"`
	To     string `json:"to"`
	Type   string `json:"type"`
}

type resourceEnvelope struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   metadata  `yaml:"metadata"`
	Spec       yaml.Node `yaml:"spec"`
}

type metadata struct {
	Name        string   `yaml:"name"`
	Workspace   string   `yaml:"workspace"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Owner       string   `yaml:"owner"`
	Tags        []string `yaml:"tags"`
}

type projectResource struct {
	Connections includeList `yaml:"connections"`
	Sources     includeList `yaml:"sources"`
	Workspaces  includeList `yaml:"workspaces"`
}

type includeList struct {
	Include []string `yaml:"include"`
}

type workspaceSpec struct {
	Uses           workspaceUses `yaml:"uses"`
	Models         includeList   `yaml:"models"`
	SemanticModels includeList   `yaml:"semanticModels"`
	Dashboards     includeList   `yaml:"dashboards"`
	Access         includeList   `yaml:"access"`
}

type workspaceUses struct {
	Sources []string `yaml:"sources"`
}

type sourceSpec struct {
	Format      string                        `yaml:"format"`
	Description string                        `yaml:"description"`
	Path        string                        `yaml:"path"`
	Connection  string                        `yaml:"connection"`
	Object      string                        `yaml:"object"`
	Options     map[string]any                `yaml:"options"`
	Fields      map[string]projectSourceField `yaml:"fields"`
}

type projectSourceField struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type projectSemanticModelSpec struct {
	BaseTable     string                       `yaml:"baseTable"`
	Tables        []string                     `yaml:"tables"`
	Relationships []semanticmodel.Relationship `yaml:"relationships"`
	Measures      projectSemanticModelMeasures `yaml:"measures"`
}

type projectSemanticModelMeasures struct {
	Defaults semanticmodel.MeasureDefaults
	Items    map[string]semanticmodel.MetricMeasure
}

type dashboardSpec struct {
	SemanticModel string                             `yaml:"semanticModel"`
	Filters       map[string]report.FilterDefinition `yaml:"filters"`
	Visuals       map[string]report.Visual           `yaml:"visuals"`
	Tables        map[string]report.TableVisual      `yaml:"tables"`
	Pages         []projectDashboardPage             `yaml:"pages"`
}

type projectModelTableSpec struct {
	Kind        string                                 `yaml:"kind"`
	Source      string                                 `yaml:"source"`
	Sources     []string                               `yaml:"sources"`
	SourceReads map[string][]string                    `yaml:"sourceReads"`
	SQL         string                                 `yaml:"sql"`
	Transform   semanticmodel.Transform                `yaml:"transform"`
	Columns     map[string]semanticmodel.ModelColumn   `yaml:"columns"`
	PrimaryKey  string                                 `yaml:"primaryKey"`
	Grain       string                                 `yaml:"grain"`
	Fields      map[string]projectModelField           `yaml:"fields"`
	Measures    map[string]semanticmodel.MetricMeasure `yaml:"measures"`
	Description string                                 `yaml:"description"`
}

type projectModelField struct {
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Expr        string `yaml:"expr"`
	Expression  string `yaml:"expression"`
	Type        string `yaml:"type"`
}

type projectDashboardPage struct {
	Name        string                 `yaml:"name"`
	Title       string                 `yaml:"title"`
	Description string                 `yaml:"description"`
	Canvas      dashboard.PageCanvas   `yaml:"canvas"`
	Grid        dashboard.PageGrid     `yaml:"grid"`
	Visuals     []dashboard.PageVisual `yaml:"visuals"`
}

type workspaceGroupSpec struct {
	Description string                     `yaml:"description"`
	Members     []workspaceGroupMemberSpec `yaml:"members"`
}

type workspaceGroupMemberSpec struct {
	PrincipalID string `yaml:"principalId"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"displayName"`
}

type workspaceRoleBindingSpec struct {
	Role    string                          `yaml:"role"`
	Subject workspaceRoleBindingSubjectSpec `yaml:"subject"`
}

type workspaceRoleBindingSubjectSpec struct {
	Kind        string `yaml:"kind"`
	PrincipalID string `yaml:"principalId"`
	Email       string `yaml:"email"`
	DisplayName string `yaml:"displayName"`
	Group       string `yaml:"group"`
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
		deploymentID := opts.DeploymentID
		workspaceID := workspace.WorkspaceID(id)
		graph, err := ExtractLineage(workspaceID, deploymentID, definition)
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

func PlanProject(projectPath string) (ProjectPlan, error) {
	project, err := LoadProject(projectPath)
	if err != nil {
		return ProjectPlan{}, err
	}
	plan := ProjectPlan{Project: project.Name}
	workspaceIDs := sortedMapKeys(project.Workspaces)
	for _, workspaceID := range workspaceIDs {
		workspaceProject := project.Workspaces[workspaceID]
		connections := workspaceConnections(project, workspaceProject)
		plan.Workspaces = append(plan.Workspaces, ProjectPlanWorkspace{
			ID:             workspaceID,
			Connections:    sortedMapKeys(connections),
			Sources:        sortedSetKeys(workspaceProject.AllowedSources),
			ModelTables:    sortedMapKeys(workspaceProject.Models),
			SemanticModels: sortedMapKeys(workspaceProject.SemanticModels),
			Dashboards:     sortedMapKeys(workspaceProject.Dashboards),
		})
	}
	return plan, nil
}

func PlanProjectAgainstGraph(projectPath, workspaceID string, active workspace.AssetGraph) (ProjectPlan, error) {
	compiled, err := CompileProject(projectPath, Options{DeploymentID: workspace.DeploymentID("plan")})
	if err != nil {
		return ProjectPlan{}, err
	}
	plan, err := PlanProject(projectPath)
	if err != nil {
		return ProjectPlan{}, err
	}
	if workspaceID != "" {
		filtered := plan.Workspaces[:0]
		for _, workspacePlan := range plan.Workspaces {
			if workspacePlan.ID == workspaceID {
				filtered = append(filtered, workspacePlan)
			}
		}
		if len(filtered) == 0 {
			return ProjectPlan{}, fmt.Errorf("project %q has no workspace %q", projectPath, workspaceID)
		}
		plan.Workspaces = filtered
	}
	for index := range plan.Workspaces {
		compiledWorkspace, ok := compiled.Workspaces[plan.Workspaces[index].ID]
		if !ok {
			continue
		}
		changes, dependencyChanges, summary := diffAssetGraphs(compiledWorkspace.Workspace.Graph, active)
		plan.Workspaces[index].Changes = changes
		plan.Workspaces[index].DependencyChanges = dependencyChanges
		plan.Workspaces[index].Summary = summary
	}
	return plan, nil
}

func diffAssetGraphs(authored, active workspace.AssetGraph) ([]ProjectPlanChange, []ProjectPlanDependencyChange, ProjectPlanSummary) {
	authoredAssets := map[workspace.AssetID]workspace.Asset{}
	activeAssets := map[workspace.AssetID]workspace.Asset{}
	for _, asset := range authored.Assets {
		authoredAssets[asset.ID] = asset
	}
	for _, asset := range active.Assets {
		activeAssets[asset.ID] = asset
	}
	impact := newPlanImpactContext(active)
	changes := []ProjectPlanChange{}
	for _, id := range sortedAssetIDs(authoredAssets) {
		asset := authoredAssets[id]
		activeAsset, ok := activeAssets[id]
		if !ok {
			changes = append(changes, projectPlanChange("add", asset, workspace.Asset{}, "not in active deployment", impact))
			continue
		}
		if activeAsset.ContentHash != asset.ContentHash {
			changes = append(changes, projectPlanChange("change", asset, activeAsset, "content hash changed", impact))
		}
	}
	for _, id := range sortedAssetIDs(activeAssets) {
		if _, ok := authoredAssets[id]; ok {
			continue
		}
		changes = append(changes, projectPlanChange("remove", activeAssets[id], workspace.Asset{}, "not in authored config", impact))
	}
	dependencyChanges := diffAssetEdges(authored.Edges, active.Edges)
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Action != changes[j].Action {
			return planActionOrder(changes[i].Action) < planActionOrder(changes[j].Action)
		}
		return changes[i].ID < changes[j].ID
	})
	sort.Slice(dependencyChanges, func(i, j int) bool {
		if dependencyChanges[i].Action != dependencyChanges[j].Action {
			return planActionOrder(dependencyChanges[i].Action) < planActionOrder(dependencyChanges[j].Action)
		}
		if dependencyChanges[i].From != dependencyChanges[j].From {
			return dependencyChanges[i].From < dependencyChanges[j].From
		}
		if dependencyChanges[i].To != dependencyChanges[j].To {
			return dependencyChanges[i].To < dependencyChanges[j].To
		}
		return dependencyChanges[i].Type < dependencyChanges[j].Type
	})
	summary := ProjectPlanSummary{DependencyChanges: len(dependencyChanges)}
	for _, change := range changes {
		switch change.Action {
		case "add":
			summary.Added++
		case "change":
			summary.Changed++
		case "remove":
			summary.Removed++
		}
		if change.Breaking {
			summary.Breaking = true
		}
		if change.MaterializationImpact {
			summary.MaterializationImpact = true
		}
		if change.AccessImpact {
			summary.AccessImpact = true
		}
	}
	for _, change := range dependencyChanges {
		if dependencyMaterializationImpact(change.Type) {
			summary.MaterializationImpact = true
		}
	}
	return changes, dependencyChanges, summary
}

type planImpactContext struct {
	activeIncomingUse map[workspace.AssetID]struct{}
	activeAssets      map[workspace.AssetID]workspace.Asset
	activeEdges       []workspace.AssetEdge
}

func newPlanImpactContext(active workspace.AssetGraph) planImpactContext {
	assets := map[workspace.AssetID]workspace.Asset{}
	for _, asset := range active.Assets {
		assets[asset.ID] = asset
	}
	return planImpactContext{
		activeIncomingUse: activeIncomingUseEdges(active.Edges),
		activeAssets:      assets,
		activeEdges:       active.Edges,
	}
}

func activeIncomingUseEdges(edges []workspace.AssetEdge) map[workspace.AssetID]struct{} {
	used := map[workspace.AssetID]struct{}{}
	for _, edge := range edges {
		if edge.Type == workspace.AssetEdgeContains {
			continue
		}
		used[edge.ToAssetID] = struct{}{}
	}
	return used
}

func projectPlanChange(action string, asset, active workspace.Asset, reason string, impact planImpactContext) ProjectPlanChange {
	breaking := action == "remove" && (breakingAssetType(asset.Type) || usedSemanticChild(asset, impact))
	if action == "change" && semanticBreakingChange(asset, active, impact) {
		breaking = true
	}
	return ProjectPlanChange{
		Action:                action,
		ID:                    string(asset.ID),
		Type:                  string(asset.Type),
		Key:                   asset.Key,
		Reason:                reason,
		Breaking:              breaking,
		MaterializationImpact: materializationAssetType(asset.Type),
		AccessImpact:          accessAssetType(asset.Type),
	}
}

func diffAssetEdges(authored, active []workspace.AssetEdge) []ProjectPlanDependencyChange {
	authoredEdges := map[planEdgeKey]struct{}{}
	activeEdges := map[planEdgeKey]struct{}{}
	for _, edge := range authored {
		authoredEdges[planEdgeKey{from: edge.FromAssetID, to: edge.ToAssetID, typ: edge.Type}] = struct{}{}
	}
	for _, edge := range active {
		activeEdges[planEdgeKey{from: edge.FromAssetID, to: edge.ToAssetID, typ: edge.Type}] = struct{}{}
	}
	changes := []ProjectPlanDependencyChange{}
	for _, key := range sortedPlanEdgeKeys(authoredEdges) {
		if _, ok := activeEdges[key]; ok {
			continue
		}
		changes = append(changes, ProjectPlanDependencyChange{
			Action: "add",
			From:   string(key.from),
			To:     string(key.to),
			Type:   string(key.typ),
		})
	}
	for _, key := range sortedPlanEdgeKeys(activeEdges) {
		if _, ok := authoredEdges[key]; ok {
			continue
		}
		changes = append(changes, ProjectPlanDependencyChange{
			Action: "remove",
			From:   string(key.from),
			To:     string(key.to),
			Type:   string(key.typ),
		})
	}
	return changes
}

type planEdgeKey struct {
	from workspace.AssetID
	to   workspace.AssetID
	typ  workspace.AssetEdgeType
}

func sortedAssetIDs(values map[workspace.AssetID]workspace.Asset) []workspace.AssetID {
	keys := make([]workspace.AssetID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedPlanEdgeKeys(values map[planEdgeKey]struct{}) []planEdgeKey {
	keys := make([]planEdgeKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		if keys[i].to != keys[j].to {
			return keys[i].to < keys[j].to
		}
		return keys[i].typ < keys[j].typ
	})
	return keys
}

func planActionOrder(action string) int {
	switch action {
	case "add":
		return 0
	case "change":
		return 1
	case "remove":
		return 2
	default:
		return 3
	}
}

func breakingAssetType(typ workspace.AssetType) bool {
	switch typ {
	case workspace.AssetTypeCatalog, workspace.AssetTypeDashboard, workspace.AssetTypeSemanticModel, workspace.AssetTypeModelTable, workspace.AssetTypeSource:
		return true
	default:
		return false
	}
}

func usedSemanticChild(asset workspace.Asset, impact planImpactContext) bool {
	switch asset.Type {
	case workspace.AssetTypeSemanticTable, workspace.AssetTypeField, workspace.AssetTypeMeasure:
		_, ok := impact.activeIncomingUse[asset.ID]
		return ok
	default:
		return false
	}
}

func materializationAssetType(typ workspace.AssetType) bool {
	switch typ {
	case workspace.AssetTypeSource, workspace.AssetTypeModelTable:
		return true
	default:
		return false
	}
}

func accessAssetType(typ workspace.AssetType) bool {
	switch typ {
	case workspace.AssetTypeWorkspaceGroup, workspace.AssetTypeWorkspaceRoleBinding:
		return true
	default:
		return false
	}
}

func semanticBreakingChange(authored, active workspace.Asset, impact planImpactContext) bool {
	switch authored.Type {
	case workspace.AssetTypeSource:
		var next, prev sourcePayloadV1
		if !decodeAssetPayload(authored, &next) || !decodeAssetPayload(active, &prev) {
			return false
		}
		return sourceFieldsBreaking(next.Fields, prev.Fields, sourceFieldUseNames(active.ID, impact))
	case workspace.AssetTypeModelTable:
		var next, prev modelTablePayloadV1
		if !decodeAssetPayload(authored, &next) || !decodeAssetPayload(active, &prev) {
			return false
		}
		return modelFieldsBreaking(next, prev, modelFieldUseNames(active, impact))
	case workspace.AssetTypeField:
		var next, prev fieldPayloadV1
		if !decodeAssetPayload(authored, &next) || !decodeAssetPayload(active, &prev) {
			return false
		}
		return fieldPayloadBreaking(next, prev)
	case workspace.AssetTypeMeasure:
		var next, prev measurePayloadV1
		if !decodeAssetPayload(authored, &next) || !decodeAssetPayload(active, &prev) {
			return false
		}
		return measurePayloadBreaking(next, prev)
	default:
		return false
	}
}

func decodeAssetPayload(asset workspace.Asset, out any) bool {
	if asset.PayloadJSON == "" {
		return false
	}
	return json.Unmarshal([]byte(asset.PayloadJSON), out) == nil
}

func sourceFieldsBreaking(next, prev map[string]sourceFieldPayloadV1, used map[string]struct{}) bool {
	for name, oldField := range prev {
		if _, ok := used["*"]; !ok {
			if _, ok := used[name]; !ok {
				continue
			}
		}
		newField, ok := next[name]
		if !ok {
			return true
		}
		if oldField.Type != "" && newField.Type != "" && oldField.Type != newField.Type {
			return true
		}
	}
	return false
}

func modelFieldsBreaking(next, prev modelTablePayloadV1, used map[string]struct{}) bool {
	for name, oldColumn := range prev.Columns {
		if _, ok := used[name]; !ok {
			continue
		}
		newColumn, ok := next.Columns[name]
		if !ok {
			return true
		}
		if oldColumn.Type != "" && newColumn.Type != "" && oldColumn.Type != newColumn.Type {
			return true
		}
	}
	for name := range prev.Fields {
		if _, ok := used[name]; !ok {
			continue
		}
		if _, ok := next.Fields[name]; !ok {
			return true
		}
	}
	return false
}

func sourceFieldUseNames(sourceID workspace.AssetID, impact planImpactContext) map[string]struct{} {
	used := map[string]struct{}{}
	for _, edge := range impact.activeEdges {
		if edge.Type != workspace.AssetEdgeReadsSource || edge.ToAssetID != sourceID {
			continue
		}
		asset, ok := impact.activeAssets[edge.FromAssetID]
		if !ok {
			continue
		}
		var table modelTablePayloadV1
		if !decodeAssetPayload(asset, &table) {
			continue
		}
		sql := table.SQL + " " + table.Transform.SQL
		if strings.Contains(sql, "*") || (strings.TrimSpace(sql) == "" && len(table.Columns) == 0) {
			used["*"] = struct{}{}
		}
		for _, column := range table.Columns {
			if column.SourceField != "" {
				used[column.SourceField] = struct{}{}
			}
			if column.Field != "" {
				used[column.Field] = struct{}{}
			}
			if column.Name != "" {
				used[column.Name] = struct{}{}
			}
		}
		for _, fieldName := range identifiersInText(sql) {
			used[fieldName] = struct{}{}
		}
	}
	return used
}

func modelFieldUseNames(modelAsset workspace.Asset, impact planImpactContext) map[string]struct{} {
	used := map[string]struct{}{}
	semanticTables := map[workspace.AssetID]struct{}{}
	for _, edge := range impact.activeEdges {
		if edge.Type == workspace.AssetEdgeUsesModelTable && edge.ToAssetID == modelAsset.ID {
			semanticTables[edge.FromAssetID] = struct{}{}
		}
	}
	if len(semanticTables) == 0 {
		return used
	}
	for _, asset := range impact.activeAssets {
		if asset.Type != workspace.AssetTypeField {
			continue
		}
		if _, ok := semanticTables[asset.ParentID]; !ok {
			continue
		}
		var field fieldPayloadV1
		if !decodeAssetPayload(asset, &field) {
			continue
		}
		addFieldPayloadUseNames(used, field)
	}
	return used
}

func addFieldPayloadUseNames(used map[string]struct{}, field fieldPayloadV1) {
	for _, value := range []string{field.Name, field.Field, field.Expression, field.Expr} {
		for _, ident := range identifiersInText(value) {
			used[ident] = struct{}{}
		}
	}
}

func identifiersInText(value string) []string {
	out := []string{}
	start := -1
	for index, r := range value {
		if isIdentifierRune(r) {
			if start == -1 {
				start = index
			}
			continue
		}
		if start != -1 {
			out = append(out, value[start:index])
			start = -1
		}
	}
	if start != -1 {
		out = append(out, value[start:])
	}
	return out
}

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func fieldPayloadBreaking(next, prev fieldPayloadV1) bool {
	if prev.Field != next.Field || prev.Table != next.Table || prev.Name != next.Name {
		return true
	}
	if prev.Type != "" && next.Type != "" && prev.Type != next.Type {
		return true
	}
	return strings.TrimSpace(prev.Expr) != strings.TrimSpace(next.Expr) || strings.TrimSpace(prev.Expression) != strings.TrimSpace(next.Expression)
}

func measurePayloadBreaking(next, prev measurePayloadV1) bool {
	if prev.Field != next.Field || prev.Table != next.Table || prev.Name != next.Name {
		return true
	}
	if strings.TrimSpace(prev.Expr) != strings.TrimSpace(next.Expr) || strings.TrimSpace(prev.Expression) != strings.TrimSpace(next.Expression) {
		return true
	}
	if prev.Time != next.Time || prev.Grain != next.Grain || !sameStringList(prev.Grains, next.Grains) {
		return true
	}
	return false
}

func dependencyMaterializationImpact(edgeType string) bool {
	switch workspace.AssetEdgeType(edgeType) {
	case workspace.AssetEdgeReadsSource, workspace.AssetEdgeUsesModelTable:
		return true
	default:
		return false
	}
}

func IsProjectConfigFile(path string) bool {
	return projectConfigFile(path)
}

func LoadProject(projectPath string) (Project, error) {
	envelope, err := readEnvelope(projectPath)
	if err != nil {
		return Project{}, err
	}
	if envelope.Kind != "Project" {
		return Project{}, resourceError(projectPath, envelopeResourceID(envelope, ""), "kind", "%s kind = %q, want Project", projectPath, envelope.Kind)
	}
	var spec projectResource
	if err := envelope.Spec.Decode(&spec); err != nil {
		return Project{}, resourceError(projectPath, envelopeResourceID(envelope, ""), "spec", "%s spec: %s", projectPath, err.Error())
	}
	baseDir := filepath.Dir(projectPath)
	project := Project{
		Name:            envelope.Metadata.Name,
		BaseDir:         baseDir,
		Connections:     map[string]semanticmodel.Connection{},
		ConnectionPaths: map[string]string{},
		Sources:         map[string]semanticmodel.Source{},
		SourcePaths:     map[string]string{},
		Workspaces:      map[string]*WorkspaceProject{},
	}
	if err := loadConnections(&project, spec.Connections.Include); err != nil {
		return Project{}, err
	}
	if err := loadSources(&project, spec.Sources.Include); err != nil {
		return Project{}, err
	}
	if err := loadWorkspaces(&project, spec.Workspaces.Include); err != nil {
		return Project{}, err
	}
	return project, validateProject(project)
}

func loadConnections(project *Project, includes []string) error {
	paths, err := expandIncludes(project.BaseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "Connection" {
			return resourceError(path, envelopeResourceID(envelope, ""), "kind", "%s kind = %q, want Connection", path, envelope.Kind)
		}
		var spec semanticmodel.Connection
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, ""), "spec", "%s spec: %s", path, err.Error())
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := project.Connections[name]; exists {
			return resourceError(path, "connection:"+name, "metadata.name", "duplicate Connection %q", name)
		}
		project.Connections[name] = spec
		project.ConnectionPaths[name] = path
	}
	return nil
}

func loadSources(project *Project, includes []string) error {
	paths, err := expandIncludes(project.BaseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "Source" {
			return resourceError(path, envelopeResourceID(envelope, ""), "kind", "%s kind = %q, want Source", path, envelope.Kind)
		}
		var spec sourceSpec
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, ""), "spec", "%s spec: %s", path, err.Error())
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := project.Sources[name]; exists {
			return resourceError(path, "source:"+name, "metadata.name", "duplicate Source %q", name)
		}
		source := semanticmodel.Source{
			Format:      spec.Format,
			Description: firstNonEmpty(spec.Description, envelope.Metadata.Description),
			Path:        spec.Path,
			Connection:  spec.Connection,
			Object:      spec.Object,
			Options:     spec.Options,
			Fields:      map[string]semanticmodel.SourceField{},
		}
		for field, cfg := range spec.Fields {
			source.Fields[field] = semanticmodel.SourceField{Type: cfg.Type, Description: cfg.Description}
		}
		project.Sources[name] = source
		project.SourcePaths[name] = path
	}
	return nil
}

func loadWorkspaces(project *Project, includes []string) error {
	paths, err := expandIncludes(project.BaseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "Workspace" {
			return resourceError(path, envelopeResourceID(envelope, ""), "kind", "%s kind = %q, want Workspace", path, envelope.Kind)
		}
		var spec workspaceSpec
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, ""), "spec", "%s spec: %s", path, err.Error())
		}
		id := envelope.Metadata.Name
		if id == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := project.Workspaces[id]; exists {
			return resourceError(path, "workspace:"+id, "metadata.name", "duplicate Workspace %q", id)
		}
		workspaceProject := &WorkspaceProject{
			ID:                    id,
			Title:                 firstNonEmpty(envelope.Metadata.Title, id),
			Description:           envelope.Metadata.Description,
			AllowedSources:        map[string]struct{}{},
			Models:                map[string]semanticmodel.Table{},
			SemanticModels:        map[string]projectSemanticModelSpec{},
			Dashboards:            map[string]*report.Dashboard{},
			AccessGroups:          map[string]workspace.WorkspaceGroup{},
			AccessRoleBindings:    map[string]workspace.WorkspaceRoleBinding{},
			ModelTitles:           map[string]string{},
			ModelDescriptions:     map[string]string{},
			DashboardTitles:       map[string]string{},
			DashboardDescriptions: map[string]string{},
			DashboardTags:         map[string][]string{},
			Path:                  path,
			ModelPaths:            map[string]string{},
			SemanticModelPaths:    map[string]string{},
			DashboardPaths:        map[string]string{},
			AccessPaths:           map[string]string{},
		}
		for _, source := range spec.Uses.Sources {
			workspaceProject.AllowedSources[source] = struct{}{}
		}
		workspaceDir := filepath.Dir(path)
		if err := loadWorkspaceModels(workspaceProject, workspaceDir, spec.Models.Include); err != nil {
			return err
		}
		if err := loadWorkspaceSemanticModels(workspaceProject, workspaceDir, spec.SemanticModels.Include); err != nil {
			return err
		}
		if err := loadWorkspaceDashboards(workspaceProject, workspaceDir, spec.Dashboards.Include); err != nil {
			return err
		}
		if err := loadWorkspaceAccess(workspaceProject, workspaceDir, spec.Access.Include); err != nil {
			return err
		}
		project.Workspaces[id] = workspaceProject
	}
	return nil
}

func loadWorkspaceModels(workspaceProject *WorkspaceProject, baseDir string, includes []string) error {
	paths, err := expandIncludes(baseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "ModelTable" {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "kind", "%s kind = %q, want ModelTable", path, envelope.Kind)
		}
		if envelope.Metadata.Workspace != "" && envelope.Metadata.Workspace != workspaceProject.ID {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "metadata.workspace", "%s workspace = %q, want %q", path, envelope.Metadata.Workspace, workspaceProject.ID)
		}
		var spec projectModelTableSpec
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "spec", "%s spec: %s", path, err.Error())
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := workspaceProject.Models[name]; exists {
			return resourceError(path, "model_table:"+workspaceProject.ID+"."+name, "metadata.name", "duplicate ModelTable %q in workspace %q", name, workspaceProject.ID)
		}
		workspaceProject.Models[name] = projectModelTable(spec)
		workspaceProject.ModelTitles[name] = envelope.Metadata.Title
		workspaceProject.ModelDescriptions[name] = envelope.Metadata.Description
		workspaceProject.ModelPaths[name] = path
	}
	return nil
}

func loadWorkspaceSemanticModels(workspaceProject *WorkspaceProject, baseDir string, includes []string) error {
	paths, err := expandIncludes(baseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "SemanticModel" {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "kind", "%s kind = %q, want SemanticModel", path, envelope.Kind)
		}
		if envelope.Metadata.Workspace != "" && envelope.Metadata.Workspace != workspaceProject.ID {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "metadata.workspace", "%s workspace = %q, want %q", path, envelope.Metadata.Workspace, workspaceProject.ID)
		}
		var spec projectSemanticModelSpec
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "spec", "%s spec: %s", path, err.Error())
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := workspaceProject.SemanticModels[name]; exists {
			return resourceError(path, "semantic_model:"+workspaceProject.ID+"."+name, "metadata.name", "duplicate SemanticModel %q in workspace %q", name, workspaceProject.ID)
		}
		workspaceProject.SemanticModels[name] = spec
		workspaceProject.ModelTitles[name] = envelope.Metadata.Title
		workspaceProject.ModelDescriptions[name] = envelope.Metadata.Description
		workspaceProject.SemanticModelPaths[name] = path
	}
	return nil
}

func loadWorkspaceDashboards(workspaceProject *WorkspaceProject, baseDir string, includes []string) error {
	paths, err := expandIncludes(baseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Kind != "Dashboard" {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "kind", "%s kind = %q, want Dashboard", path, envelope.Kind)
		}
		if envelope.Metadata.Workspace != "" && envelope.Metadata.Workspace != workspaceProject.ID {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "metadata.workspace", "%s workspace = %q, want %q", path, envelope.Metadata.Workspace, workspaceProject.ID)
		}
		var spec dashboardSpec
		if err := envelope.Spec.Decode(&spec); err != nil {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "spec", "%s spec: %s", path, err.Error())
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		if _, exists := workspaceProject.Dashboards[name]; exists {
			return resourceError(path, "dashboard:"+workspaceProject.ID+"."+name, "metadata.name", "duplicate Dashboard %q in workspace %q", name, workspaceProject.ID)
		}
		dashboard := &report.Dashboard{
			ID:            name,
			Title:         envelope.Metadata.Title,
			Description:   envelope.Metadata.Description,
			SemanticModel: spec.SemanticModel,
			Filters:       spec.Filters,
			Visuals:       spec.Visuals,
			Tables:        spec.Tables,
			Pages:         projectDashboardPages(spec.Pages),
		}
		workspaceProject.Dashboards[name] = dashboard
		workspaceProject.DashboardTitles[name] = envelope.Metadata.Title
		workspaceProject.DashboardDescriptions[name] = envelope.Metadata.Description
		workspaceProject.DashboardTags[name] = append([]string{}, envelope.Metadata.Tags...)
		workspaceProject.DashboardPaths[name] = path
	}
	return nil
}

func loadWorkspaceAccess(workspaceProject *WorkspaceProject, baseDir string, includes []string) error {
	paths, err := expandIncludes(baseDir, includes)
	if err != nil {
		return err
	}
	for _, path := range paths {
		envelope, err := readEnvelope(path)
		if err != nil {
			return err
		}
		if envelope.Metadata.Workspace != "" && envelope.Metadata.Workspace != workspaceProject.ID {
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "metadata.workspace", "%s workspace = %q, want %q", path, envelope.Metadata.Workspace, workspaceProject.ID)
		}
		name := envelope.Metadata.Name
		if name == "" {
			return resourceError(path, "", "metadata.name", "%s metadata.name is required", path)
		}
		switch envelope.Kind {
		case "WorkspaceGroup":
			var spec workspaceGroupSpec
			if err := envelope.Spec.Decode(&spec); err != nil {
				return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "spec", "%s spec: %s", path, err.Error())
			}
			if _, exists := workspaceProject.AccessGroups[name]; exists {
				return resourceError(path, "workspace_group:"+workspaceProject.ID+"."+name, "metadata.name", "duplicate WorkspaceGroup %q in workspace %q", name, workspaceProject.ID)
			}
			workspaceProject.AccessGroups[name] = projectWorkspaceGroup(name, spec)
			workspaceProject.AccessPaths["WorkspaceGroup:"+name] = path
		case "WorkspaceRoleBinding":
			var spec workspaceRoleBindingSpec
			if err := envelope.Spec.Decode(&spec); err != nil {
				return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "spec", "%s spec: %s", path, err.Error())
			}
			if _, exists := workspaceProject.AccessRoleBindings[name]; exists {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "metadata.name", "duplicate WorkspaceRoleBinding %q in workspace %q", name, workspaceProject.ID)
			}
			workspaceProject.AccessRoleBindings[name] = projectWorkspaceRoleBinding(name, spec)
			workspaceProject.AccessPaths["WorkspaceRoleBinding:"+name] = path
		default:
			return resourceError(path, envelopeResourceID(envelope, workspaceProject.ID), "kind", "%s kind = %q, want WorkspaceGroup or WorkspaceRoleBinding", path, envelope.Kind)
		}
	}
	return nil
}

func projectModelTable(spec projectModelTableSpec) semanticmodel.Table {
	table := semanticmodel.Table{
		Kind:        spec.Kind,
		Source:      spec.Source,
		Sources:     append([]string{}, spec.Sources...),
		SourceReads: copyStringSliceMap(spec.SourceReads),
		SQL:         spec.SQL,
		Transform:   spec.Transform,
		Columns:     copyModelColumns(spec.Columns),
		PrimaryKey:  spec.PrimaryKey,
		Grain:       spec.Grain,
		Dimensions:  map[string]semanticmodel.MetricDimension{},
		Measures:    copyMeasures(spec.Measures),
		Description: spec.Description,
	}
	for name, field := range spec.Fields {
		table.Dimensions[name] = semanticmodel.MetricDimension{
			Label:       field.Label,
			Description: field.Description,
			Type:        field.Type,
			Expr:        field.Expr,
			Expression:  field.Expression,
		}
		if field.Type != "" {
			if table.Columns == nil {
				table.Columns = map[string]semanticmodel.ModelColumn{}
			}
			column := table.Columns[name]
			column.Type = field.Type
			column.Description = firstNonEmpty(column.Description, field.Description)
			table.Columns[name] = column
		}
	}
	return table
}

func projectDashboardPages(pages []projectDashboardPage) []dashboard.Page {
	out := make([]dashboard.Page, 0, len(pages))
	for _, page := range pages {
		out = append(out, dashboard.Page{
			ID:          page.Name,
			Title:       page.Title,
			Description: page.Description,
			Canvas:      page.Canvas,
			Grid:        page.Grid,
			Visuals:     page.Visuals,
		})
	}
	return out
}

func projectWorkspaceGroup(name string, spec workspaceGroupSpec) workspace.WorkspaceGroup {
	group := workspace.WorkspaceGroup{
		ID:          name,
		Name:        name,
		Description: spec.Description,
		Members:     make([]workspace.WorkspaceGroupMember, 0, len(spec.Members)),
	}
	for _, member := range spec.Members {
		group.Members = append(group.Members, workspace.WorkspaceGroupMember{
			PrincipalID: strings.TrimSpace(member.PrincipalID),
			Email:       strings.TrimSpace(member.Email),
			DisplayName: strings.TrimSpace(member.DisplayName),
		})
	}
	sort.SliceStable(group.Members, func(i, j int) bool {
		return accessMemberSortKey(group.Members[i]) < accessMemberSortKey(group.Members[j])
	})
	return group
}

func projectWorkspaceRoleBinding(name string, spec workspaceRoleBindingSpec) workspace.WorkspaceRoleBinding {
	return workspace.WorkspaceRoleBinding{
		ID:   name,
		Name: name,
		Role: strings.TrimSpace(spec.Role),
		Subject: workspace.WorkspaceRoleBindingSubject{
			Kind:        strings.TrimSpace(spec.Subject.Kind),
			PrincipalID: strings.TrimSpace(spec.Subject.PrincipalID),
			Email:       strings.TrimSpace(spec.Subject.Email),
			DisplayName: strings.TrimSpace(spec.Subject.DisplayName),
			Group:       strings.TrimSpace(spec.Subject.Group),
		},
	}
}

func validateProject(project Project) error {
	for connectionName, connection := range project.Connections {
		if _, err := connection.Validate(connectionName); err != nil {
			return resourceError(project.ConnectionPaths[connectionName], "connection:"+connectionName, "spec", "Connection %q %s", connectionName, err.Error())
		}
	}
	for sourceName, source := range project.Sources {
		if _, ok := project.Connections[source.Connection]; !ok {
			return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec.connection", "Source %q references unknown Connection %q", sourceName, source.Connection)
		}
		if source.Path != "" && source.Format == "" {
			format, ok := semanticmodel.InferFormat(source.Path)
			if !ok {
				return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec.format", "Source %q path %q requires format", sourceName, source.Path)
			}
			source.Format = format
		}
		if err := source.Validate(localSourceName(sourceName), project.Connections); err != nil {
			return resourceError(project.SourcePaths[sourceName], "source:"+sourceName, "spec", "Source %q %s", sourceName, err.Error())
		}
	}
	for _, workspaceProject := range project.Workspaces {
		for source := range workspaceProject.AllowedSources {
			if _, ok := project.Sources[source]; !ok {
				return resourceError(workspaceProject.Path, "workspace:"+workspaceProject.ID, "spec.uses.sources", "Workspace %q allows unknown Source %q", workspaceProject.ID, source)
			}
		}
		if len(workspaceProject.SemanticModels) == 0 {
			return resourceError(workspaceProject.Path, "workspace:"+workspaceProject.ID, "spec.semanticModels", "Workspace %q requires SemanticModel resources", workspaceProject.ID)
		}
		for tableName, table := range workspaceProject.Models {
			for _, source := range table.Sources {
				if _, ok := workspaceProject.AllowedSources[source]; !ok {
					return resourceError(workspaceProject.ModelPaths[tableName], "model_table:"+workspaceProject.ID+"."+tableName, "spec.sources", "ModelTable %q.%q reads source %q outside uses.sources", workspaceProject.ID, tableName, source)
				}
			}
			if table.Source != "" {
				if _, ok := workspaceProject.AllowedSources[table.Source]; !ok {
					return resourceError(workspaceProject.ModelPaths[tableName], "model_table:"+workspaceProject.ID+"."+tableName, "spec.source", "ModelTable %q.%q reads source %q outside uses.sources", workspaceProject.ID, tableName, table.Source)
				}
			}
			if err := validateProjectTableSources(workspaceProject.ID, tableName, workspaceProject.ModelPaths[tableName], table); err != nil {
				return err
			}
		}
		for name, dashboard := range workspaceProject.Dashboards {
			if _, ok := workspaceProject.SemanticModels[dashboard.SemanticModel]; !ok {
				return resourceError(workspaceProject.DashboardPaths[name], "dashboard:"+workspaceProject.ID+"."+name, "spec.semanticModel", "Dashboard %q.%q references unknown SemanticModel %q", workspaceProject.ID, name, dashboard.SemanticModel)
			}
		}
		if err := validateWorkspaceAccess(workspaceProject); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkspaceAccess(workspaceProject *WorkspaceProject) error {
	validRoles := map[string]struct{}{
		access.RoleOwner:    {},
		access.RoleAdmin:    {},
		access.RoleDeployer: {},
		access.RoleEditor:   {},
		access.RoleViewer:   {},
	}
	for name, group := range workspaceProject.AccessGroups {
		for index, member := range group.Members {
			if member.PrincipalID == "" && member.Email == "" {
				return resourceError(workspaceProject.AccessPaths["WorkspaceGroup:"+name], "workspace_group:"+workspaceProject.ID+"."+name, fmt.Sprintf("spec.members[%d]", index), "WorkspaceGroup %q.%q member requires principalId or email", workspaceProject.ID, name)
			}
		}
	}
	for name, binding := range workspaceProject.AccessRoleBindings {
		path := workspaceProject.AccessPaths["WorkspaceRoleBinding:"+name]
		if _, ok := validRoles[binding.Role]; !ok {
			return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.role", "WorkspaceRoleBinding %q.%q references unknown role %q", workspaceProject.ID, name, binding.Role)
		}
		switch binding.Subject.Kind {
		case string(access.SubjectGroup):
			if binding.Subject.Group == "" {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.group", "WorkspaceRoleBinding %q.%q group subject requires group", workspaceProject.ID, name)
			}
			if _, ok := workspaceProject.AccessGroups[binding.Subject.Group]; !ok {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.group", "WorkspaceRoleBinding %q.%q references unknown WorkspaceGroup %q", workspaceProject.ID, name, binding.Subject.Group)
			}
		case string(access.SubjectPrincipal):
			if binding.Subject.PrincipalID == "" && binding.Subject.Email == "" {
				return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject", "WorkspaceRoleBinding %q.%q principal subject requires principalId or email", workspaceProject.ID, name)
			}
		default:
			return resourceError(path, "workspace_role_binding:"+workspaceProject.ID+"."+name, "spec.subject.kind", "WorkspaceRoleBinding %q.%q has unsupported subject kind %q", workspaceProject.ID, name, binding.Subject.Kind)
		}
	}
	return nil
}

func validateProjectTableSources(workspaceID, tableName, path string, table semanticmodel.Table) error {
	sql := strings.TrimSpace(table.Transform.SQL)
	if sql == "" {
		sql = strings.TrimSpace(table.SQL)
	}
	if sql == "" {
		return nil
	}
	declared := append([]string{}, table.Sources...)
	if table.Source != "" {
		declared = append(declared, table.Source)
	}
	sort.Strings(declared)
	inferred, rawRefs, unqualifiedRefs := (&semanticmodel.Model{}).SQLSourceRefs(sql)
	if len(rawRefs) > 0 {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sql", "ModelTable %q.%q SQL must reference sources through source.<name>; raw.<name> is internal", workspaceID, tableName)
	}
	if len(unqualifiedRefs) > 0 {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sql", "ModelTable %q.%q SQL must reference sources through source.<name>; found unqualified relation %q", workspaceID, tableName, unqualifiedRefs[0])
	}
	if !sameStringList(declared, inferred) {
		return resourceError(path, "model_table:"+workspaceID+"."+tableName, "spec.sources", "ModelTable %q.%q SQL source references %v do not match declared sources %v", workspaceID, tableName, inferred, declared)
	}
	return nil
}

func (workspaceProject *WorkspaceProject) definition(project Project) (*workspace.Definition, error) {
	sourceAliases := map[string]string{}
	sourceIDs := map[string]string{}
	for source := range workspaceProject.AllowedSources {
		alias := localSourceName(source)
		if existing, exists := sourceIDs[alias]; exists && existing != source {
			return nil, fmt.Errorf("workspace %q sources %q and %q resolve to duplicate runtime alias %q", workspaceProject.ID, existing, source, alias)
		}
		sourceAliases[source] = alias
		sourceIDs[alias] = source
	}
	catalog := workspace.Catalog{
		Workspace: workspace.CatalogWorkspace{
			ID:          workspaceProject.ID,
			Title:       workspaceProject.Title,
			Description: workspaceProject.Description,
		},
		SemanticModels: []workspace.CatalogModel{},
		Dashboards:     []workspace.CatalogDashboard{},
	}
	definition := &workspace.Definition{
		Catalog:    catalog,
		Models:     map[string]*semanticmodel.Model{},
		Dashboards: workspaceProject.Dashboards,
		Access: workspace.AccessPolicy{
			Groups:       copyWorkspaceGroups(workspaceProject.AccessGroups),
			RoleBindings: copyWorkspaceRoleBindings(workspaceProject.AccessRoleBindings),
		},
		BaseDir:     project.BaseDir,
		SourceIDs:   sourceIDs,
		SourceFiles: workspaceProject.sourceFiles(project),
	}
	for _, modelName := range sortedMapKeys(workspaceProject.SemanticModels) {
		semanticSpec := workspaceProject.SemanticModels[modelName]
		model, err := workspaceProject.semanticModel(project, modelName, semanticSpec, sourceAliases)
		if err != nil {
			return nil, err
		}
		definition.Models[modelName] = model
		definition.Catalog.SemanticModels = append(definition.Catalog.SemanticModels, workspace.CatalogModel{
			ID:          modelName,
			Title:       model.Title,
			Description: model.Description,
		})
	}
	for name := range workspaceProject.Dashboards {
		dashboard := workspaceProject.Dashboards[name]
		if err := ValidateDashboard(dashboard, definition.Models); err != nil {
			return nil, resourceError(workspaceProject.DashboardPaths[name], "dashboard:"+workspaceProject.ID+"."+name, "spec", "loading dashboard %q: %s", name, err.Error())
		}
		definition.Catalog.Dashboards = append(definition.Catalog.Dashboards, workspace.CatalogDashboard{
			ID:          name,
			Title:       firstNonEmpty(workspaceProject.DashboardTitles[name], dashboard.Title),
			Description: workspaceProject.DashboardDescriptions[name],
			Tags:        append([]string{}, workspaceProject.DashboardTags[name]...),
		})
	}
	sort.Slice(definition.Catalog.Dashboards, func(i, j int) bool {
		return definition.Catalog.Dashboards[i].ID < definition.Catalog.Dashboards[j].ID
	})
	return definition, nil
}

func (workspaceProject *WorkspaceProject) sourceFiles(project Project) map[string]string {
	sourceFiles := map[string]string{}
	workspaceKey := func(name string) string {
		return workspaceProject.ID + "." + name
	}
	sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeCatalog, workspaceProject.ID))] = workspaceProject.Path
	for name, path := range project.ConnectionPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeConnection, name))] = path
	}
	for name, path := range project.SourcePaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeSource, name))] = path
	}
	for name, path := range workspaceProject.ModelPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeModelTable, workspaceKey(name)))] = path
	}
	for name, path := range workspaceProject.SemanticModelPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeSemanticModel, workspaceKey(name)))] = path
	}
	for name, path := range workspaceProject.DashboardPaths {
		sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeDashboard, workspaceKey(name)))] = path
	}
	for accessKey, path := range workspaceProject.AccessPaths {
		kind, name, ok := strings.Cut(accessKey, ":")
		if !ok {
			continue
		}
		switch kind {
		case "WorkspaceGroup":
			sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeWorkspaceGroup, workspaceKey(name)))] = path
		case "WorkspaceRoleBinding":
			sourceFiles[string(workspace.NewAssetID(workspace.AssetTypeWorkspaceRoleBinding, workspaceKey(name)))] = path
		}
	}
	return sourceFiles
}

func (workspaceProject *WorkspaceProject) semanticModel(project Project, modelName string, semanticSpec projectSemanticModelSpec, sourceAliases map[string]string) (*semanticmodel.Model, error) {
	model := &semanticmodel.Model{
		Name:          modelName,
		Title:         firstNonEmpty(workspaceProject.ModelTitles[modelName], modelName),
		Description:   workspaceProject.ModelDescriptions[modelName],
		Connections:   workspaceConnections(project, workspaceProject),
		Sources:       map[string]semanticmodel.Source{},
		Tables:        copyTables(workspaceProject.Models),
		BaseTable:     semanticSpec.BaseTable,
		Relationships: append([]semanticmodel.Relationship{}, semanticSpec.Relationships...),
		Measures:      map[string]semanticmodel.MetricMeasure{},
	}
	model.DefaultConnection = firstConnectionName(model.Connections)
	for source, alias := range sourceAliases {
		model.Sources[alias] = project.Sources[source]
	}
	model.Tables = translatedTablesForRuntime(model.Tables, sourceAliases)
	if err := applySemanticModelSpec(model, semanticSpec); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec", "%s", err.Error())
	}
	if err := model.Validate(); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec", "%s", err.Error())
	}
	if _, err := analyticsmaterialize.ModelTableOrder(model); err != nil {
		return nil, resourceError(workspaceProject.SemanticModelPaths[modelName], "semantic_model:"+workspaceProject.ID+"."+modelName, "spec.tables", "%s", err.Error())
	}
	return model, nil
}

func translatedTablesForRuntime(in map[string]semanticmodel.Table, sourceAliases map[string]string) map[string]semanticmodel.Table {
	out := make(map[string]semanticmodel.Table, len(in))
	for name, table := range in {
		if alias, ok := sourceAliases[table.Source]; ok {
			table.Source = alias
		}
		for index, source := range table.Sources {
			if alias, ok := sourceAliases[source]; ok {
				table.Sources[index] = alias
			}
		}
		table.SQL = rewriteSourceSQLForRuntime(table.SQL, sourceAliases)
		table.Transform.SQL = rewriteSourceSQLForRuntime(table.Transform.SQL, sourceAliases)
		out[name] = table
	}
	return out
}

func rewriteSourceSQLForRuntime(sql string, sourceAliases map[string]string) string {
	for global, local := range sourceAliases {
		sql = strings.ReplaceAll(sql, `source."`+global+`"`, "source."+local)
		sql = strings.ReplaceAll(sql, "source."+global, "source."+local)
	}
	return sql
}

func localSourceName(sourceID string) string {
	var builder strings.Builder
	for index, char := range sourceID {
		valid := char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || index > 0 && char >= '0' && char <= '9'
		if valid {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('_')
	}
	out := builder.String()
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "source_" + out
	}
	return out
}

func workspaceConnections(project Project, workspaceProject *WorkspaceProject) map[string]semanticmodel.Connection {
	out := map[string]semanticmodel.Connection{}
	for sourceID := range workspaceProject.AllowedSources {
		source, ok := project.Sources[sourceID]
		if !ok {
			continue
		}
		connection, ok := project.Connections[source.Connection]
		if !ok {
			continue
		}
		out[source.Connection] = connection
	}
	return out
}

func applySemanticModelSpec(model *semanticmodel.Model, spec projectSemanticModelSpec) error {
	if spec.BaseTable == "" {
		return fmt.Errorf("SemanticModel %q requires baseTable", model.Name)
	}
	if len(spec.Tables) == 0 {
		return fmt.Errorf("SemanticModel %q requires tables", model.Name)
	}
	tables := map[string]semanticmodel.Table{}
	for _, tableName := range spec.Tables {
		table, ok := model.Tables[tableName]
		if !ok {
			return fmt.Errorf("SemanticModel %q references unknown ModelTable %q", model.Name, tableName)
		}
		tables[tableName] = table
	}
	defaults := spec.Measures.Defaults
	measures := map[string]semanticmodel.MetricMeasure{}
	for name, measure := range spec.Measures.Items {
		if measure.Expression == "" {
			measure.Expression = measure.Expr
		}
		measure.Table = firstNonEmpty(measure.Table, defaults.Table)
		measure.Grain = firstNonEmpty(measure.Grain, defaults.Grain)
		measure.Time = firstNonEmpty(measure.Time, defaults.Time)
		if len(measure.Grains) == 0 {
			measure.Grains = append([]string{}, defaults.Grains...)
		}
		measure.Field = name
		measure.Name = name
		measures[name] = measure
	}
	model.Tables = tables
	model.BaseTable = spec.BaseTable
	model.Relationships = append([]semanticmodel.Relationship{}, spec.Relationships...)
	model.Measures = measures
	return nil
}

func (m *projectSemanticModelMeasures) UnmarshalYAML(value *yaml.Node) error {
	m.Items = map[string]semanticmodel.MetricMeasure{}
	if value == nil || value.Kind == yaml.ScalarNode && value.Tag == "!!null" {
		return nil
	}
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("semantic model measures must be a mapping")
	}
	for index := 0; index+1 < len(value.Content); index += 2 {
		key := value.Content[index].Value
		item := value.Content[index+1]
		if key == "defaults" {
			if err := item.Decode(&m.Defaults); err != nil {
				return err
			}
			continue
		}
		var measure semanticmodel.MetricMeasure
		if item.Kind != yaml.ScalarNode || item.Tag != "!!null" {
			if err := item.Decode(&measure); err != nil {
				return err
			}
		}
		m.Items[key] = measure
	}
	return nil
}

func readEnvelope(path string) (resourceEnvelope, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return resourceEnvelope{}, err
	}
	if kind, ok := schemaKindForEnvelope(content); ok {
		if err := configschema.ValidateBytes(kind, path, content); err != nil {
			return resourceEnvelope{}, annotateSchemaError(err, path, resourceIDForHeader(content, ""), "spec")
		}
	}
	var envelope resourceEnvelope
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&envelope); err != nil {
		return resourceEnvelope{}, fmt.Errorf("%s: %w", path, err)
	}
	if envelope.APIVersion != projectAPIVersion {
		return resourceEnvelope{}, resourceError(path, envelopeResourceID(envelope, ""), "apiVersion", "%s apiVersion = %q, want %q", path, envelope.APIVersion, projectAPIVersion)
	}
	if envelope.Kind == "" {
		return resourceEnvelope{}, resourceError(path, envelopeResourceID(envelope, ""), "kind", "%s kind is required", path)
	}
	return envelope, nil
}

func resourceIDForHeader(content []byte, fallbackWorkspace string) string {
	var envelope resourceEnvelope
	if err := yaml.Unmarshal(content, &envelope); err != nil {
		return ""
	}
	return envelopeResourceID(envelope, fallbackWorkspace)
}

func envelopeResourceID(envelope resourceEnvelope, fallbackWorkspace string) string {
	name := envelope.Metadata.Name
	if name == "" {
		return ""
	}
	workspaceID := firstNonEmpty(envelope.Metadata.Workspace, fallbackWorkspace)
	switch envelope.Kind {
	case "Project":
		return "project:" + name
	case "Connection":
		return "connection:" + name
	case "Source":
		return "source:" + name
	case "Workspace":
		return "workspace:" + name
	case "ModelTable":
		if workspaceID == "" {
			return ""
		}
		return "model_table:" + workspaceID + "." + name
	case "SemanticModel":
		if workspaceID == "" {
			return ""
		}
		return "semantic_model:" + workspaceID + "." + name
	case "Dashboard":
		if workspaceID == "" {
			return ""
		}
		return "dashboard:" + workspaceID + "." + name
	case "WorkspaceGroup":
		if workspaceID == "" {
			return ""
		}
		return "workspace_group:" + workspaceID + "." + name
	case "WorkspaceRoleBinding":
		if workspaceID == "" {
			return ""
		}
		return "workspace_role_binding:" + workspaceID + "." + name
	default:
		return ""
	}
}

func schemaKindForEnvelope(content []byte) (configschema.Kind, bool) {
	var header struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &header); err != nil {
		return "", false
	}
	if header.APIVersion != projectAPIVersion {
		return "", false
	}
	switch header.Kind {
	case "Project":
		return configschema.KindProject, true
	case "Connection":
		return configschema.KindConnection, true
	case "Source":
		return configschema.KindSource, true
	case "Workspace":
		return configschema.KindWorkspace, true
	case "WorkspaceGroup":
		return configschema.KindWorkspaceGroup, true
	case "WorkspaceRoleBinding":
		return configschema.KindWorkspaceRoleBinding, true
	case "ModelTable":
		return configschema.KindModelTable, true
	case "SemanticModel":
		return configschema.KindSemanticModelResource, true
	case "Dashboard":
		return configschema.KindDashboardResource, true
	default:
		return "", false
	}
}

func projectConfigFile(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var envelope struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &envelope); err != nil {
		return false
	}
	return envelope.APIVersion == projectAPIVersion && envelope.Kind == "Project"
}

func expandIncludes(baseDir string, includes []string) ([]string, error) {
	var paths []string
	for _, pattern := range includes {
		if strings.TrimSpace(pattern) == "" {
			return nil, fmt.Errorf("include pattern is required")
		}
		if filepath.IsAbs(pattern) {
			return nil, fmt.Errorf("include pattern %q must be relative", pattern)
		}
		if strings.Contains(filepath.ToSlash(pattern), "**") {
			return nil, fmt.Errorf("include pattern %q uses unsupported ** glob", pattern)
		}
		for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(pattern)), "/") {
			if part == ".." {
				return nil, fmt.Errorf("include pattern %q escapes project boundary", pattern)
			}
		}
		matches, err := filepath.Glob(filepath.Join(baseDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("include pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("include pattern %q matched no files", pattern)
		}
		sort.Strings(matches)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				return nil, fmt.Errorf("include pattern %q matched directory %s", pattern, match)
			}
			ext := strings.ToLower(filepath.Ext(match))
			if ext != ".yaml" && ext != ".yml" {
				return nil, fmt.Errorf("include pattern %q matched non-YAML file %s", pattern, match)
			}
		}
		paths = append(paths, matches...)
	}
	return paths, nil
}

func firstConnectionName(connections map[string]semanticmodel.Connection) string {
	names := make([]string, 0, len(connections))
	for name := range connections {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func copyConnections(in map[string]semanticmodel.Connection) map[string]semanticmodel.Connection {
	out := make(map[string]semanticmodel.Connection, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyTables(in map[string]semanticmodel.Table) map[string]semanticmodel.Table {
	out := make(map[string]semanticmodel.Table, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, value := range in {
		out[key] = append([]string{}, value...)
	}
	return out
}

func copyModelColumns(in map[string]semanticmodel.ModelColumn) map[string]semanticmodel.ModelColumn {
	if in == nil {
		return nil
	}
	out := make(map[string]semanticmodel.ModelColumn, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyMeasures(in map[string]semanticmodel.MetricMeasure) map[string]semanticmodel.MetricMeasure {
	if in == nil {
		return nil
	}
	out := make(map[string]semanticmodel.MetricMeasure, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyWorkspaceGroups(in map[string]workspace.WorkspaceGroup) map[string]workspace.WorkspaceGroup {
	out := make(map[string]workspace.WorkspaceGroup, len(in))
	for key, value := range in {
		value.Members = append([]workspace.WorkspaceGroupMember{}, value.Members...)
		out[key] = value
	}
	return out
}

func copyWorkspaceRoleBindings(in map[string]workspace.WorkspaceRoleBinding) map[string]workspace.WorkspaceRoleBinding {
	out := make(map[string]workspace.WorkspaceRoleBinding, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func accessMemberSortKey(member workspace.WorkspaceGroupMember) string {
	return member.Email + "\x00" + member.PrincipalID + "\x00" + member.DisplayName
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStringList(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
