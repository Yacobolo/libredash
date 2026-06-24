package compiler

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace"
)

var lineageFieldRefPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\b`)

func ExtractLineage(workspaceID workspace.WorkspaceID, deploymentID workspace.DeploymentID, definition *workspace.Definition) (workspace.AssetGraph, error) {
	graph := workspace.AssetGraph{}
	byKey := map[string]workspace.AssetID{}
	seenEdges := map[string]struct{}{}
	add := func(typ workspace.AssetType, key string, parentID workspace.AssetID, title, description string, content any) (workspace.AssetID, error) {
		asset, err := workspace.NewAsset(workspaceID, deploymentID, typ, key, parentID, title, description, content)
		if err != nil {
			return "", err
		}
		graph.Assets = append(graph.Assets, asset)
		byKey[string(typ)+":"+key] = asset.ID
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

	catalogID, err := add(workspace.AssetTypeCatalog, string(workspaceID), "", workspaceTitle(definition.Catalog.Workspace.Title), definition.Catalog.Workspace.Description, definition.Catalog)
	if err != nil {
		return workspace.AssetGraph{}, err
	}
	for _, modelEntry := range definition.Catalog.SemanticModels {
		model := definition.Models[modelEntry.ID]
		for _, connectionName := range sortedMapKeys(model.Connections) {
			connection := model.Connections[connectionName]
			id, err := add(workspace.AssetTypeConnection, modelEntry.ID+"."+connectionName, catalogID, connectionName, connection.Description, connection)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
		}
		for _, sourceName := range sortedMapKeys(model.Sources) {
			source := model.Sources[sourceName]
			id, err := add(workspace.AssetTypeSource, modelEntry.ID+"."+sourceName, catalogID, sourceName, source.Description, source)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
			connectionID, err := assetID(workspace.AssetTypeConnection, modelEntry.ID+"."+source.Connection)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, connectionID, workspace.AssetEdgeUsesConnection)
		}
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			id, err := add(workspace.AssetTypeModelTable, modelEntry.ID+"."+tableName, catalogID, tableName, table.Description, table)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(catalogID, id, workspace.AssetEdgeContains)
			for _, sourceName := range table.SourceDependencies {
				sourceID, err := assetID(workspace.AssetTypeSource, modelEntry.ID+"."+sourceName)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, sourceID, workspace.AssetEdgeReadsSource)
			}
		}
		modelID, err := add(workspace.AssetTypeSemanticModel, modelEntry.ID, catalogID, modelEntry.Title, modelEntry.Description, model)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, modelID, workspace.AssetEdgeContains)
		for _, tableName := range sortedMapKeys(model.Tables) {
			table := model.Tables[tableName]
			semanticTableID, err := add(workspace.AssetTypeSemanticTable, modelEntry.ID+"."+tableName, modelID, tableName, table.Description, table)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, semanticTableID, workspace.AssetEdgeContains)
			modelTableID, err := assetID(workspace.AssetTypeModelTable, modelEntry.ID+"."+tableName)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(semanticTableID, modelTableID, workspace.AssetEdgeUsesModelTable)
			if table.PrimaryKey != "" {
				field := table.Dimensions[table.PrimaryKey]
				field.Field = tableName + "." + table.PrimaryKey
				field.Table = tableName
				field.Name = table.PrimaryKey
				fieldID, err := add(workspace.AssetTypeField, modelEntry.ID+"."+tableName+"."+table.PrimaryKey, semanticTableID, dimensionLabel(table.PrimaryKey, field.Label), field.Description, field)
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
				fieldID, err := add(workspace.AssetTypeField, modelEntry.ID+"."+tableName+"."+fieldName, semanticTableID, dimensionLabel(fieldName, field.Label), field.Description, field)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(semanticTableID, fieldID, workspace.AssetEdgeContains)
			}
		}
		for _, relationship := range model.Relationships {
			id, err := add(workspace.AssetTypeRelationship, modelEntry.ID+"."+relationship.ID, modelID, relationship.ID, relationship.Description, relationship)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			for _, endpoint := range []string{relationship.From, relationship.To} {
				fieldID, err := fieldAssetID(modelEntry.ID, model, endpoint, assetID)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
		for _, measureName := range sortedMapKeys(model.Measures) {
			measure := model.Measures[measureName]
			id, err := add(workspace.AssetTypeMeasure, modelEntry.ID+"."+measureName, modelID, measureLabel(measureName, measure.Label), measure.Description, measure)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(modelID, id, workspace.AssetEdgeContains)
			tableID, err := assetID(workspace.AssetTypeSemanticTable, modelEntry.ID+"."+measure.Table)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(id, tableID, workspace.AssetEdgeUsesSemanticTable)
			for _, ref := range lineageMeasureFieldRefs(model, measure) {
				fieldID, err := assetID(workspace.AssetTypeField, modelEntry.ID+"."+ref)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(id, fieldID, workspace.AssetEdgeUsesField)
			}
		}
	}
	for _, reportEntry := range definition.Catalog.Dashboards {
		report := definition.Dashboards[reportEntry.ID]
		reportID, err := add(workspace.AssetTypeDashboard, reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, report)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(catalogID, reportID, workspace.AssetEdgeContains)
		modelID, err := assetID(workspace.AssetTypeSemanticModel, report.SemanticModel)
		if err != nil {
			return workspace.AssetGraph{}, err
		}
		edge(reportID, modelID, workspace.AssetEdgeUsesSemanticModel)
		model := definition.Models[report.SemanticModel]
		addSemanticTableUse := func(fromID workspace.AssetID, tableName string) error {
			if tableName == "" {
				return nil
			}
			tableID, err := assetID(workspace.AssetTypeSemanticTable, report.SemanticModel+"."+tableName)
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
					fieldID, err := assetID(workspace.AssetTypeField, report.SemanticModel+"."+fieldRef)
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
			measureID, err := assetID(workspace.AssetTypeMeasure, report.SemanticModel+"."+measure.Name)
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
				fieldID, err := assetID(workspace.AssetTypeField, report.SemanticModel+"."+dimension.Field)
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
			measureID, err := assetID(workspace.AssetTypeMeasure, report.SemanticModel+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, workspace.AssetEdgeUsesMeasure)
			return nil
		}
		for _, filterName := range sortedMapKeys(report.Filters) {
			filter := report.Filters[filterName]
			filterID, err := add(workspace.AssetTypeFilter, report.ID+"."+filterName, reportID, filter.Label, filter.Description, filter)
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
			visualID, err := add(workspace.AssetTypeVisual, report.ID+"."+visualName, reportID, visual.Title, visual.Description, visual)
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
			tableID, err := add(workspace.AssetTypeTable, report.ID+"."+tableName, reportID, table.Title, table.Description, table)
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
			pageID, err := add(workspace.AssetTypePage, report.ID+"."+page.ID, reportID, page.Title, page.Description, page)
			if err != nil {
				return workspace.AssetGraph{}, err
			}
			edge(reportID, pageID, workspace.AssetEdgeContains)
			for index, item := range page.Visuals {
				itemKey := item.ID
				if itemKey == "" {
					itemKey = strconv.Itoa(index)
				}
				itemID, err := add(workspace.AssetTypePageItem, report.ID+"."+page.ID+"."+itemKey, pageID, pageItemTitle(item), item.Description, item)
				if err != nil {
					return workspace.AssetGraph{}, err
				}
				edge(pageID, itemID, workspace.AssetEdgeContains)
				if item.Visual != "" {
					visualID, err := assetID(workspace.AssetTypeVisual, report.ID+"."+item.Visual)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, visualID, workspace.AssetEdgeUsesVisual)
				}
				if item.Table != "" {
					tableID, err := assetID(workspace.AssetTypeTable, report.ID+"."+item.Table)
					if err != nil {
						return workspace.AssetGraph{}, err
					}
					edge(itemID, tableID, workspace.AssetEdgeUsesTable)
				}
				if item.Filter != "" {
					filterID, err := assetID(workspace.AssetTypeFilter, report.ID+"."+item.Filter)
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

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fieldAssetID(modelID string, model *semanticmodel.Model, ref string, assetID func(workspace.AssetType, string) (workspace.AssetID, error)) (workspace.AssetID, error) {
	dimension, err := model.ResolveRelationshipEndpoint(ref)
	if err != nil {
		return "", err
	}
	return assetID(workspace.AssetTypeField, modelID+"."+dimension.Field)
}

func lineageMeasureFieldRefs(model *semanticmodel.Model, measure semanticmodel.MetricMeasure) []string {
	return lineageExpressionFieldRefs(model, measure.SQLExpression())
}

func lineageExpressionFieldRefs(model *semanticmodel.Model, expression string) []string {
	seen := map[string]struct{}{}
	for _, match := range lineageFieldRefPattern.FindAllStringSubmatch(expression, -1) {
		ref := match[1] + "." + match[2]
		dimension, err := model.ResolveRelationshipEndpoint(ref)
		if err != nil {
			continue
		}
		seen[dimension.Field] = struct{}{}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func pageItemTitle(item dashboard.PageVisual) string {
	if item.Title != "" {
		return item.Title
	}
	if item.ID != "" {
		return item.ID
	}
	return item.Kind
}

func dimensionLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}

func measureLabel(name, label string) string {
	if label != "" {
		return label
	}
	return name
}
