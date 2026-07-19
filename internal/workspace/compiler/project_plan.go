package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Yacobolo/libredash/internal/workspace"
)

type ProjectPlan struct {
	Project    string                 `json:"project"`
	Workspaces []ProjectPlanWorkspace `json:"workspaces"`
}

type ProjectPlanWorkspace struct {
	ID                     string                        `json:"id"`
	Connections            []string                      `json:"connections"`
	Sources                []string                      `json:"sources"`
	ModelTables            []string                      `json:"modelTables"`
	SemanticModels         []string                      `json:"semanticModels"`
	Dashboards             []string                      `json:"dashboards"`
	WorkspaceGroups        []string                      `json:"workspaceGroups"`
	WorkspaceRoleBindings  []string                      `json:"workspaceRoleBindings"`
	Grants                 []string                      `json:"grants"`
	DataPolicies           []string                      `json:"dataPolicies"`
	WorkspaceAgentPolicies []string                      `json:"workspaceAgentPolicies"`
	Changes                []ProjectPlanChange           `json:"changes,omitempty"`
	DependencyChanges      []ProjectPlanDependencyChange `json:"dependencyChanges,omitempty"`
	Summary                ProjectPlanSummary            `json:"summary,omitempty"`
}

type ProjectPlanSummary struct {
	Added                 int  `json:"added,omitempty"`
	Changed               int  `json:"changed,omitempty"`
	Removed               int  `json:"removed,omitempty"`
	DependencyChanges     int  `json:"dependencyChanges,omitempty"`
	Breaking              bool `json:"breaking,omitempty"`
	MaterializationImpact bool `json:"materializationImpact,omitempty"`
	AccessImpact          bool `json:"accessImpact,omitempty"`
	AgentPolicyImpact     bool `json:"agentPolicyImpact,omitempty"`
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
	AgentPolicyImpact     bool   `json:"agentPolicyImpact,omitempty"`
}

type ProjectPlanDependencyChange struct {
	Action   string `json:"action"`
	From     string `json:"from"`
	To       string `json:"to"`
	Type     string `json:"type"`
	Breaking bool   `json:"breaking,omitempty"`
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
			ID:                     workspaceID,
			Connections:            sortedMapKeys(connections),
			Sources:                sortedSetKeys(workspaceProject.AllowedSources),
			ModelTables:            sortedMapKeys(workspaceProject.Models),
			SemanticModels:         sortedMapKeys(workspaceProject.SemanticModels),
			Dashboards:             sortedMapKeys(workspaceProject.Dashboards),
			WorkspaceGroups:        sortedMapKeys(workspaceProject.AccessGroups),
			WorkspaceRoleBindings:  sortedMapKeys(workspaceProject.AccessRoleBindings),
			Grants:                 sortedMapKeys(workspaceProject.AccessGrants),
			DataPolicies:           sortedMapKeys(workspaceProject.AccessDataPolicies),
			WorkspaceAgentPolicies: sortedMapKeys(workspaceProject.AgentPolicies),
		})
	}
	return plan, nil
}

func PlanProjectAgainstGraph(projectPath, workspaceID string, active workspace.AssetGraph) (ProjectPlan, error) {
	compiled, err := CompileProject(projectPath, Options{ServingStateID: workspace.ServingStateID("plan")})
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
		authoredAssets[asset.ID] = assetForPlanComparison(asset)
	}
	for _, asset := range active.Assets {
		activeAssets[asset.ID] = assetForPlanComparison(asset)
	}
	impact := newPlanImpactContext(active)
	changes := []ProjectPlanChange{}
	for _, id := range sortedAssetIDs(authoredAssets) {
		asset := authoredAssets[id]
		activeAsset, ok := activeAssets[id]
		if !ok {
			changes = append(changes, projectPlanChange("add", asset, workspace.Asset{}, "not in active serving state", impact))
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
	dependencyChanges := diffAssetEdges(authored.Edges, active.Edges, impact)
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
		if change.AgentPolicyImpact {
			summary.AgentPolicyImpact = true
		}
	}
	for _, change := range dependencyChanges {
		if change.Breaking {
			summary.Breaking = true
		}
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
		AgentPolicyImpact:     agentPolicyAssetType(asset.Type),
	}
}

func assetForPlanComparison(asset workspace.Asset) workspace.Asset {
	if asset.PayloadJSON == "" || !assetHasRuntimeSchema(asset.Type) {
		return asset
	}
	payloadBytes, err := normalizedRuntimeSchemaPayloadJSON(asset.Type, []byte(asset.PayloadJSON))
	if err != nil {
		return asset
	}
	contentHash, err := workspace.AssetContentHash(workspace.AssetHashInput{
		Type:          asset.Type,
		Key:           asset.Key,
		ParentID:      asset.ParentID,
		Title:         asset.Title,
		Description:   asset.Description,
		PayloadSchema: asset.PayloadSchema,
		PayloadJSON:   json.RawMessage(payloadBytes),
	})
	if err != nil {
		return asset
	}
	asset.ContentHash = contentHash
	return asset
}

func assetHasRuntimeSchema(typ workspace.AssetType) bool {
	switch typ {
	case workspace.AssetTypeSource, workspace.AssetTypeModelTable, workspace.AssetTypeSemanticTable, workspace.AssetTypeSemanticModel:
		return true
	default:
		return false
	}
}

func normalizedRuntimeSchemaPayloadJSON(typ workspace.AssetType, payloadJSON []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, err
	}
	switch typ {
	case workspace.AssetTypeSource, workspace.AssetTypeModelTable, workspace.AssetTypeSemanticTable:
		delete(payload, "Schema")
	case workspace.AssetTypeSemanticModel:
		removeNestedRuntimeSchemas(payload, "Sources")
		removeNestedRuntimeSchemas(payload, "Tables")
		removeNestedRuntimeSchemas(payload, "Models")
	}
	return json.Marshal(payload)
}

func removeNestedRuntimeSchemas(payload map[string]any, key string) {
	values, ok := payload[key].(map[string]any)
	if !ok {
		return
	}
	for _, child := range values {
		childMap, ok := child.(map[string]any)
		if !ok {
			continue
		}
		delete(childMap, "Schema")
	}
}

func diffAssetEdges(authored, active []workspace.AssetEdge, impact planImpactContext) []ProjectPlanDependencyChange {
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
			Action:   "remove",
			From:     string(key.from),
			To:       string(key.to),
			Type:     string(key.typ),
			Breaking: dependencyBreakingImpact(key.typ, key.from, key.to, impact),
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

func agentPolicyAssetType(typ workspace.AssetType) bool {
	return typ == workspace.AssetTypeWorkspaceAgentPolicy
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
	if prev.Field != next.Field || prev.Fact != next.Fact || prev.Name != next.Name {
		return true
	}
	if prev.Aggregation != next.Aggregation || prev.Empty != next.Empty || prev.InputField != next.InputField {
		return true
	}
	if strings.TrimSpace(prev.InputExpression) != strings.TrimSpace(next.InputExpression) {
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

func dependencyBreakingImpact(edgeType workspace.AssetEdgeType, fromID, toID workspace.AssetID, impact planImpactContext) bool {
	switch edgeType {
	case workspace.AssetEdgeUsesSemanticModel,
		workspace.AssetEdgeUsesSemanticTable,
		workspace.AssetEdgeUsesField,
		workspace.AssetEdgeFiltersField,
		workspace.AssetEdgeUsesMeasure,
		workspace.AssetEdgeUsesVisual,
		workspace.AssetEdgeUsesFilter:
		return true
	case workspace.AssetEdgeReadsSource:
		_, used := impact.activeIncomingUse[fromID]
		return used
	case workspace.AssetEdgeUsesModelTable:
		_, used := impact.activeIncomingUse[fromID]
		return used
	default:
		return false
	}
}
