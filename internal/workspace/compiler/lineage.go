package compiler

import (
	"fmt"
	"strconv"
	"strings"

	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func ExtractLineage(workspaceID workspace.WorkspaceID, deploymentID workspace.DeploymentID, definition *workspace.Definition) (workspace.AssetGraph, error) {
	graph := workspace.AssetGraph{}
	byKey := map[string]workspace.AssetID{}
	sourceFilesByID := map[workspace.AssetID]string{}
	seenEdges := map[string]struct{}{}
	workspaceKey := func(name string) string {
		return string(workspaceID) + "." + name
	}
	sourceKey := func(runtimeName string) string {
		if definition.SourceIDs != nil {
			if globalName, ok := definition.SourceIDs[runtimeName]; ok {
				return globalName
			}
		}
		return runtimeName
	}
	sourceTitle := func(sourceID string) string {
		if _, title, ok := strings.Cut(sourceID, "."); ok && title != "" {
			return title
		}
		return sourceID
	}
	add := func(typ workspace.AssetType, key string, parentID workspace.AssetID, title, description string, payload any) (workspace.AssetID, error) {
		lookupKey := string(typ) + ":" + key
		if existing := byKey[lookupKey]; existing != "" {
			return existing, nil
		}
		id := workspace.NewAssetID(typ, key)
		sourceFile := ""
		if definition.SourceFiles != nil {
			sourceFile = definition.SourceFiles[string(id)]
		}
		if sourceFile == "" && parentID != "" {
			sourceFile = sourceFilesByID[parentID]
		}
		asset, err := workspace.NewAssetWithSourceFile(workspaceID, deploymentID, typ, key, parentID, title, description, sourceFile, workspace.PayloadSchemaForAssetType(typ), payload)
		if err != nil {
			return "", err
		}
		graph.Assets = append(graph.Assets, asset)
		byKey[lookupKey] = asset.ID
		sourceFilesByID[asset.ID] = sourceFile
		return asset.ID, nil
	}
	edge := func(fromID, toID workspace.AssetID, typ workspace.AssetEdgeType) {
		if fromID == "" || toID == "" {
			return
		}
		key := string(fromID) + "|" + string(toID) + "|" + string(typ)
		if _, ok := seenEdges[key]; ok {
			return
		}
		seenEdges[key] = struct{}{}
		graph.Edges = append(graph.Edges, workspace.NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ))
	}
	assetID := func(typ workspace.AssetType, key string) (workspace.AssetID, error) {
		id := byKey[string(typ)+":"+key]
		if id == "" {
			return "", fmt.Errorf("missing extracted %s asset %q", typ, key)
		}
		return id, nil
	}

	catalogID, err := add(workspace.AssetTypeCatalog, string(workspaceID), "", workspaceTitle(definition.Catalog.Workspace.Title, string(workspaceID)), definition.Catalog.Workspace.Description, catalogPayload(definition))
	if err != nil {
		return workspace.AssetGraph{}, err
	}
	for _, groupName := range sortedMapKeys(definition.Access.Groups) {
		group := definition.Access.Groups[groupName]
		id, err := add(workspace.AssetTypeWorkspaceGroup, workspaceKey(groupName), catalogID, group.Name, group.Description, workspaceGroupPayload(group))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, id, workspace.AssetEdgeContains)
	}
	for _, bindingName := range sortedMapKeys(definition.Access.RoleBindings) {
		binding := definition.Access.RoleBindings[bindingName]
		id, err := add(workspace.AssetTypeWorkspaceRoleBinding, workspaceKey(bindingName), catalogID, binding.Name, "", workspaceRoleBindingPayload(binding))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, id, workspace.AssetEdgeContains)
		if binding.Subject.Kind == "group" && binding.Subject.Group != "" {
			groupID, err := assetID(workspace.AssetTypeWorkspaceGroup, workspaceKey(binding.Subject.Group))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, groupID, workspace.AssetEdgeUsesGroup)
		}
	}
	for _, policyName := range sortedMapKeys(definition.AgentPolicies) {
		policy := definition.AgentPolicies[policyName]
		id, err := add(workspace.AssetTypeWorkspaceAgentPolicy, workspaceKey(policyName), catalogID, policy.Name, "", workspaceAgentPolicyPayload(policy))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, id, workspace.AssetEdgeContains)
	}
	for _, modelEntry := range definition.Catalog.SemanticModels {
		model := definition.Models[modelEntry.ID]
		for _, connectionName := range sortedMapKeys(model.Connections) {
			connection := model.Connections[connectionName]
			id, err := add(workspace.AssetTypeConnection, connectionName, catalogID, connectionName, connection.Description, connectionPayload(connection))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
		}
		for _, sourceName := range sortedMapKeys(model.Sources) {
			source := model.Sources[sourceName]
			globalSourceName := sourceKey(sourceName)
			id, err := add(workspace.AssetTypeSource, globalSourceName, catalogID, sourceTitle(globalSourceName), source.Description, sourcePayload(source))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
			connectionID, err := assetID(workspace.AssetTypeConnection, source.Connection)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, connectionID, workspace.AssetEdgeUsesConnection)
		}
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			id, err := add(workspace.AssetTypeModelTable, workspaceKey(tableName), catalogID, tableName, table.Description, modelTablePayload(table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
		}
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			id, err := assetID(workspace.AssetTypeModelTable, workspaceKey(tableName))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			for _, sourceName := range table.SourceDependencies {
				sourceID, err := assetID(workspace.AssetTypeSource, sourceKey(sourceName))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, sourceID, workspace.AssetEdgeReadsSource)
			}
			for _, dependency := range table.ModelDependencies {
				dependencyID, err := assetID(workspace.AssetTypeModelTable, workspaceKey(dependency))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, dependencyID, workspace.AssetEdgeUsesModelTable)
			}
		}
		modelKey := workspaceKey(modelEntry.ID)
		modelID, err := add(workspace.AssetTypeSemanticModel, modelKey, catalogID, modelEntry.Title, modelEntry.Description, semanticModelPayload(model))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, modelID, workspace.AssetEdgeContains)
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			semanticTableKey := modelKey + "." + tableName
			semanticTableID, err := add(workspace.AssetTypeSemanticTable, semanticTableKey, modelID, tableName, table.Description, semanticTablePayload(tableName, table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, semanticTableID, workspace.AssetEdgeContains)
			modelTableID, err := assetID(workspace.AssetTypeModelTable, workspaceKey(tableName))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(semanticTableID, modelTableID, workspace.AssetEdgeUsesModelTable)
			if table.PrimaryKey != "" {
				field := table.Dimensions[table.PrimaryKey]
				field.Field = tableName + "." + table.PrimaryKey
				field.Table = tableName
				field.Name = table.PrimaryKey
				fieldID, err := add(workspace.AssetTypeField, semanticTableKey+"."+table.PrimaryKey, semanticTableID, dimensionLabel(table.PrimaryKey, field.Label), field.Description, fieldPayload(field))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(semanticTableID, fieldID, workspace.AssetEdgeContains)
			}
			for _, fieldName := range sortedMapKeys(table.Dimensions) {
				field := table.Dimensions[fieldName]
				if fieldName == table.PrimaryKey {
					continue
				}
				fieldID, err := add(workspace.AssetTypeField, semanticTableKey+"."+fieldName, semanticTableID, dimensionLabel(fieldName, field.Label), field.Description, fieldPayload(field))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(semanticTableID, fieldID, workspace.AssetEdgeContains)
			}
		}
		for _, relationship := range model.Relationships {
			id, err := add(workspace.AssetTypeRelationship, modelKey+"."+relationship.ID, modelID, relationship.ID, relationship.Description, relationshipPayload(relationship))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			for _, endpoint := range []string{relationship.From, relationship.To} {
				fieldID, err := fieldAssetID(modelKey, model, endpoint, assetID)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
		for _, measureName := range sortedMapKeys(model.Measures) {
			measure := model.Measures[measureName]
			id, err := add(workspace.AssetTypeMeasure, modelKey+"."+measureName, modelID, measureLabel(measureName, measure.Label), measure.Description, measurePayload(measure))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			tableID, err := assetID(workspace.AssetTypeSemanticTable, modelKey+"."+measure.Table)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, tableID, workspace.AssetEdgeUsesSemanticTable)
			for _, ref := range lineageMeasureFieldRefs(model, measure) {
				fieldID, err := assetID(workspace.AssetTypeField, modelKey+"."+ref)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
	}
	for _, reportEntry := range definition.Catalog.Dashboards {
		report := definition.Dashboards[reportEntry.ID]
		reportKey := workspaceKey(reportEntry.ID)
		reportID, err := add(workspace.AssetTypeDashboard, reportKey, catalogID, reportEntry.Title, reportEntry.Description, dashboardPayload(*report, reportEntry.Tags))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, reportID, workspace.AssetEdgeContains)
		modelKey := workspaceKey(report.SemanticModel)
		modelID, err := assetID(workspace.AssetTypeSemanticModel, modelKey)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(reportID, modelID, workspace.AssetEdgeUsesSemanticModel)
		model := definition.Models[report.SemanticModel]
		addSemanticTableUse := func(fromID workspace.AssetID, tableName string) error {
			if tableName == "" {
				return nil
			}
			tableID, err := assetID(workspace.AssetTypeSemanticTable, modelKey+"."+tableName)
			if err != nil {
				return err
			}
			edge(fromID, tableID, workspace.AssetEdgeUsesSemanticTable)
			return nil
		}
		addMeasureUse := func(fromID workspace.AssetID, ref reportdef.FieldRef) error {
			if ref.Measure.Expression != "" || ref.Measure.Expr != "" {
				if err := addSemanticTableUse(fromID, ref.Measure.Table); err != nil {
					return err
				}
				for _, fieldRef := range lineageExpressionFieldRefs(model, ref.Measure.SQLExpression()) {
					fieldID, err := assetID(workspace.AssetTypeField, modelKey+"."+fieldRef)
					if err != nil {
						return err
					}
					edge(fromID, fieldID, workspace.AssetEdgeUsesField)
				}
				return nil
			}
			measure, err := model.ResolveMeasure(ref.Field)
			if err != nil {
				return err
			}
			measureID, err := assetID(workspace.AssetTypeMeasure, modelKey+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, workspace.AssetEdgeUsesMeasure)
			return nil
		}
		addFieldUse := func(fromID workspace.AssetID, ref string, edgeType workspace.AssetEdgeType) error {
			if ref == "" {
				return nil
			}
			if dimension, err := model.ResolveDimension(ref); err == nil {
				fieldID, err := assetID(workspace.AssetTypeField, modelKey+"."+dimension.Field)
				if err != nil {
					return err
				}
				edge(fromID, fieldID, edgeType)
				return nil
			}
			measure, err := model.ResolveMeasure(ref)
			if err != nil {
				return err
			}
			measureID, err := assetID(workspace.AssetTypeMeasure, modelKey+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, workspace.AssetEdgeUsesMeasure)
			return nil
		}
		for _, filterName := range sortedMapKeys(report.Filters) {
			filter := report.Filters[filterName]
			filterID, err := add(workspace.AssetTypeFilter, reportKey+"."+filterName, reportID, filter.Label, filter.Description, filterPayload(filter))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, filterID, workspace.AssetEdgeContains)
			if err := addFieldUse(filterID, filter.Dimension, workspace.AssetEdgeFiltersField); err != nil {
				return workspace.AssetGraph{}, err
			}
		}
		for _, visualName := range sortedMapKeys(report.Visuals) {
			visual := report.Visuals[visualName]
			visualID, err := add(workspace.AssetTypeVisual, reportKey+"."+visualName, reportID, visual.Title, visual.Description, visualPayload(visual))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, visualID, workspace.AssetEdgeContains)
			for _, measure := range visual.Query.Measures {
				if err := addMeasureUse(visualID, measure); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, dimension := range visual.Query.Dimensions {
				if err := addFieldUse(visualID, dimension.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			if !visual.Query.Series.IsZero() {
				if err := addFieldUse(visualID, visual.Query.Series.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			if visual.Query.Time.Field != "" {
				if err := addFieldUse(visualID, visual.Query.Time.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
		}
		for _, tableName := range sortedMapKeys(report.Tables) {
			table := report.Tables[tableName]
			tableID, err := add(workspace.AssetTypeTable, reportKey+"."+tableName, reportID, table.Title, table.Description, tableVisualPayload(table))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, tableID, workspace.AssetEdgeContains)
			for _, column := range table.DataColumns {
				if err := addFieldUse(tableID, column.Field, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, row := range table.Rows {
				if err := addFieldUse(tableID, row, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, dimension := range table.ColumnDims {
				if err := addFieldUse(tableID, dimension, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, measure := range table.Query.Measures {
				if err := addMeasureUse(tableID, measure); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
		}
		for _, page := range report.Pages {
			pageID, err := add(workspace.AssetTypePage, reportKey+"."+page.ID, reportID, page.Title, page.Description, pagePayload(page))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, pageID, workspace.AssetEdgeContains)
			for index, item := range page.Visuals {
				itemKey := item.ID
				if itemKey == "" {
					itemKey = strconv.Itoa(index)
				}
				itemID, err := add(workspace.AssetTypePageItem, reportKey+"."+page.ID+"."+itemKey, pageID, pageItemTitle(item), item.Description, pageItemPayload(item))
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(pageID, itemID, workspace.AssetEdgeContains)
				if item.Visual != "" {
					visualID, err := assetID(workspace.AssetTypeVisual, reportKey+"."+item.Visual)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, visualID, workspace.AssetEdgeUsesVisual)
				}
				if item.Table != "" {
					tableID, err := assetID(workspace.AssetTypeTable, reportKey+"."+item.Table)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, tableID, workspace.AssetEdgeUsesTable)
				}
				if item.Filter != "" {
					filterID, err := assetID(workspace.AssetTypeFilter, reportKey+"."+item.Filter)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, filterID, workspace.AssetEdgeUsesFilter)
				}
			}
		}
	}
	return graph, nil
}
