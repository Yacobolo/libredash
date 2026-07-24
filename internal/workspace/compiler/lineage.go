package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/Yacobolo/leapview/internal/workspace"
)

func ExtractLineage(workspaceID workspace.WorkspaceID, servingStateID workspace.ServingStateID, definition *workspace.Definition) (workspace.AssetGraph, error) {
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
		asset, err := workspace.NewAssetWithSourceFile(workspaceID, servingStateID, typ, key, parentID, title, description, sourceFile, workspace.PayloadSchemaForAssetType(typ), payload)
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
		graph.Edges = append(graph.Edges, workspace.NewAssetEdge(workspaceID, servingStateID, fromID, toID, typ))
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
		for _, dimensionName := range sortedMapKeys(model.Dimensions) {
			dimension := model.Dimensions[dimensionName]
			logical := semanticmodel.MetricDimension{
				Field: dimensionName, Name: dimensionName, Label: dimension.Label,
				Description: dimension.Description, Type: dimension.Type,
			}
			dimensionID, err := add(workspace.AssetTypeField, modelKey+"."+dimensionName, modelID, dimensionLabel(dimensionName, dimension.Label), dimension.Description, fieldPayload(logical))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, dimensionID, workspace.AssetEdgeContains)
			bindings := make([]string, 0, len(dimension.Bindings))
			for _, binding := range dimension.Bindings {
				bindings = append(bindings, binding.Field)
			}
			sort.Strings(bindings)
			for _, binding := range bindings {
				fieldID, err := assetID(workspace.AssetTypeField, modelKey+"."+binding)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(dimensionID, fieldID, workspace.AssetEdgeUsesField)
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
			tableID, err := assetID(workspace.AssetTypeSemanticTable, modelKey+"."+measure.Fact)
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
		for _, metricName := range sortedMapKeys(model.Metrics) {
			metric := model.Metrics[metricName]
			id, err := add(workspace.AssetTypeMeasure, modelKey+"."+metricName, modelID, measureLabel(metricName, metric.Label), metric.Description, metricMeasurePayload(metric))
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
		}
		for _, metricName := range sortedMapKeys(model.Metrics) {
			metric := model.Metrics[metricName]
			metricID, err := assetID(workspace.AssetTypeMeasure, modelKey+"."+metricName)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			expression, err := semanticmodel.ParseExpression(metric.Expression)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			for _, ref := range expression.References() {
				dependencyID, err := assetID(workspace.AssetTypeMeasure, modelKey+"."+ref)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(metricID, dependencyID, workspace.AssetEdgeUsesMeasure)
			}
		}
	}
	for _, pipelineName := range sortedMapKeys(definition.RefreshPipelines) {
		pipeline := definition.RefreshPipelines[pipelineName]
		pipelineID, err := add(workspace.AssetTypeRefreshPipeline, workspaceKey(pipelineName), catalogID, pipeline.Name, "", refreshPipelinePayload(pipeline))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, pipelineID, workspace.AssetEdgeContains)
		modelID, err := assetID(workspace.AssetTypeSemanticModel, workspaceKey(pipeline.SemanticModel))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(pipelineID, modelID, workspace.AssetEdgeRefreshesSemanticModel)
	}
	for _, reportEntry := range definition.Catalog.Dashboards {
		compiledReport := definition.Dashboards[reportEntry.ID]
		reportKey := workspaceKey(reportEntry.ID)
		reportID, err := add(workspace.AssetTypeDashboard, reportKey, catalogID, reportEntry.Title, reportEntry.Description, dashboardPayload(compiledReport, reportEntry.Tags))
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, reportID, workspace.AssetEdgeContains)
		modelKey := workspaceKey(compiledReport.SemanticModel)
		modelID, err := assetID(workspace.AssetTypeSemanticModel, modelKey)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(reportID, modelID, workspace.AssetEdgeUsesSemanticModel)
		model := definition.Models[compiledReport.SemanticModel]
		addMeasureUse := func(fromID workspace.AssetID, fieldID string) error {
			if metric, ok := model.Metrics[fieldID]; ok {
				metricID, err := assetID(workspace.AssetTypeMeasure, modelKey+"."+metric.Name)
				if err != nil {
					return err
				}
				edge(fromID, metricID, workspace.AssetEdgeUsesMeasure)
				return nil
			}
			measure, err := model.ResolveMeasure(fieldID)
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
			if _, ok := model.Dimensions[ref]; ok {
				fieldID, err := assetID(workspace.AssetTypeField, modelKey+"."+ref)
				if err != nil {
					return err
				}
				edge(fromID, fieldID, edgeType)
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
		for _, filterName := range sortedMapKeys(compiledReport.FilterDefinitions) {
			filter := compiledReport.FilterDefinitions[filterName]
			filterID, err := add(workspace.AssetTypeFilter, reportKey+"."+filterName, reportID, filter.Label, filter.Description, filter)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, filterID, workspace.AssetEdgeContains)
			if err := addFieldUse(filterID, filter.Field, workspace.AssetEdgeFiltersField); err != nil {
				return workspace.AssetGraph{}, err
			}
		}
		for _, visualName := range sortedMapKeys(compiledReport.Visualizations) {
			compiledVisual := compiledReport.Visualizations[visualName]
			visualID, err := add(workspace.AssetTypeVisual, reportKey+"."+visualName, reportID, visualizationTitle(compiledVisual), "", compiledVisual)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, visualID, workspace.AssetEdgeContains)
			fields, measures := visualizationLineageBindings(compiledVisual.Query)
			for _, measure := range measures {
				if err := addMeasureUse(visualID, measure.FieldID); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			for _, field := range fields {
				if err := addFieldUse(visualID, field.FieldID, workspace.AssetEdgeUsesField); err != nil {
					return workspace.AssetGraph{}, err
				}
			}
			base, err := visualizationir.SpecificationBase(compiledVisual.Spec)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			for _, interaction := range base.Interactions {
				for _, mapping := range interaction.Mappings {
					if err := addFieldUse(visualID, mapping.TargetFieldID, workspace.AssetEdgeFiltersField); err != nil {
						return workspace.AssetGraph{}, err
					}
				}
			}
			if geographic, ok := compiledVisual.Spec.Value.(*visualizationir.GeographicVisualizationSpec); ok {
				for _, interaction := range geographic.SpatialInteractions {
					for _, mapping := range []visualizationir.VisualizationSpatialFieldMapping{interaction.Latitude, interaction.Longitude} {
						if err := addFieldUse(visualID, mapping.TargetFieldID, workspace.AssetEdgeFiltersField); err != nil {
							return workspace.AssetGraph{}, err
						}
					}
				}
			}
		}
		for _, page := range compiledReport.Pages {
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
				if item.Kind == "slicer" {
					binding := compiledReport.FilterBindings[item.Binding.ID]
					if item.Binding.Scope == "page" {
						binding = page.FilterBindings[item.Binding.ID]
					}
					filterID, err := assetID(workspace.AssetTypeFilter, reportKey+"."+binding.Filter)
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

func visualizationTitle(definition visualizationdefinition.Definition) string {
	base, err := visualizationir.SpecificationBase(definition.Spec)
	if err != nil || base.Title == "" {
		return definition.ID
	}
	return base.Title
}

func visualizationLineageBindings(query visualizationdefinition.QueryBinding) (fields, measures []visualizationdefinition.FieldBinding) {
	switch query.Kind {
	case visualizationdefinition.QueryAggregate:
		fields = append(fields, query.Aggregate.Dimensions...)
		if query.Aggregate.Series != nil {
			fields = append(fields, *query.Aggregate.Series)
		}
		if query.Aggregate.Time != nil {
			fields = append(fields, visualizationdefinition.FieldBinding{FieldID: query.Aggregate.Time.FieldID, Alias: query.Aggregate.Time.Alias})
		}
		measures = append(measures, query.Aggregate.Measures...)
	case visualizationdefinition.QueryDetail:
		fields = append(fields, query.Detail.Fields...)
	case visualizationdefinition.QueryMatrix:
		fields = append(fields, query.Matrix.Rows...)
		fields = append(fields, query.Matrix.Columns...)
		measures = append(measures, query.Matrix.Measures...)
	case visualizationdefinition.QueryPivot:
		fields = append(fields, query.Pivot.Rows...)
		fields = append(fields, query.Pivot.Columns...)
		measures = append(measures, query.Pivot.Measures...)
	case visualizationdefinition.QueryCustom:
		fields = append(fields, query.Custom.Fields...)
	case visualizationdefinition.QuerySpatial:
		fields = append(fields, query.Spatial.Dimensions...)
		if query.Spatial.Series != nil {
			fields = append(fields, *query.Spatial.Series)
		}
		if query.Spatial.Time != nil {
			fields = append(fields, visualizationdefinition.FieldBinding{FieldID: query.Spatial.Time.FieldID, Alias: query.Spatial.Time.Alias})
		}
		measures = append(measures, query.Spatial.Measures...)
	}
	return fields, measures
}
