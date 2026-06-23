package deploy

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func ExtractAssets(workspaceID, deploymentID string, workspace *semantic.Workspace) ([]platform.Asset, []platform.AssetEdge, error) {
	assets := []platform.Asset{}
	edges := []platform.AssetEdge{}
	byKey := map[string]string{}
	seenEdges := map[string]struct{}{}
	add := func(typ, key, parentID, title, description string, content any) (string, error) {
		asset, err := platform.NewAsset(workspaceID, deploymentID, typ, key, parentID, title, description, content)
		if err != nil {
			return "", err
		}
		assets = append(assets, asset)
		byKey[typ+":"+key] = asset.ID
		return asset.ID, nil
	}
	edge := func(fromID, toID, typ string) {
		if fromID == "" || toID == "" {
			return
		}
		key := fromID + "|" + toID + "|" + typ
		if _, ok := seenEdges[key]; ok {
			return
		}
		seenEdges[key] = struct{}{}
		edges = append(edges, platform.NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ))
	}
	assetID := func(typ, key string) (string, error) {
		id := byKey[typ+":"+key]
		if id == "" {
			return "", fmt.Errorf("missing extracted %s asset %q", typ, key)
		}
		return id, nil
	}

	catalogID, err := add("catalog", workspaceID, "", workspaceTitle(workspace.Catalog.Workspace.Title), workspace.Catalog.Workspace.Description, workspace.Catalog)
	if err != nil {
		return nil, nil, err
	}
	for _, modelEntry := range workspace.Catalog.SemanticModels {
		model := workspace.Models[modelEntry.ID]
		modelID, err := add("semantic_model", modelEntry.ID, catalogID, modelEntry.Title, modelEntry.Description, model)
		if err != nil {
			return nil, nil, err
		}
		edge(catalogID, modelID, "contains")
		for connectionName, connection := range model.Connections {
			id, err := add("connection", modelEntry.ID+"."+connectionName, modelID, connectionName, connectionDescription(connection), connection)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
		}
		for sourceName, source := range model.Sources {
			id, err := add("source", modelEntry.ID+"."+sourceName, modelID, sourceName, source.Description(), source)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			connectionID, err := assetID("connection", modelEntry.ID+"."+source.Connection)
			if err != nil {
				return nil, nil, err
			}
			edge(id, connectionID, "uses_connection")
		}
		for tableName, table := range model.Tables {
			id, err := add("model_table", modelEntry.ID+"."+tableName, modelID, tableName, table.Description, table)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			for _, sourceName := range table.SourceDependencies {
				sourceID, err := assetID("source", modelEntry.ID+"."+sourceName)
				if err != nil {
					return nil, nil, err
				}
				edge(id, sourceID, "reads_source")
			}
			for fieldName, field := range table.Dimensions {
				fieldID, err := add("field", modelEntry.ID+"."+tableName+"."+fieldName, id, dimensionLabel(fieldName, field.Label), "", field)
				if err != nil {
					return nil, nil, err
				}
				edge(id, fieldID, "contains")
			}
		}
		for measureName, measure := range model.Measures {
			id, err := add("measure", modelEntry.ID+"."+measureName, modelID, measureLabel(measureName, measure.Label), measure.Description, measure)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			tableID, err := assetID("model_table", modelEntry.ID+"."+measure.Table)
			if err != nil {
				return nil, nil, err
			}
			edge(id, tableID, "uses_model_table")
		}
	}
	for _, reportEntry := range workspace.Catalog.Dashboards {
		report := workspace.Dashboards[reportEntry.ID]
		reportID, err := add("dashboard", reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, report)
		if err != nil {
			return nil, nil, err
		}
		modelID, err := assetID("semantic_model", report.SemanticModel)
		if err != nil {
			return nil, nil, err
		}
		edge(reportID, modelID, "uses_semantic_model")
		model := workspace.Models[report.SemanticModel]
		usedTables := map[string]bool{}
		addTableUse := func(tableName string) error {
			if tableName == "" || usedTables[tableName] {
				return nil
			}
			tableID, err := assetID("model_table", report.SemanticModel+"."+tableName)
			if err != nil {
				return err
			}
			edge(reportID, tableID, "uses_model_table")
			usedTables[tableName] = true
			return nil
		}
		addMeasureUse := func(ref semantic.FieldRef) error {
			if ref.Measure.Expression != "" || ref.Measure.Expr != "" {
				return addTableUse(ref.Measure.Table)
			}
			measure, err := model.ResolveMeasure(ref.Field)
			if err != nil {
				return err
			}
			measureID, err := assetID("measure", report.SemanticModel+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(reportID, measureID, "uses_measure")
			return addTableUse(measure.Table)
		}
		addFieldUse := func(fromID string, ref string, edgeType string) error {
			if ref == "" {
				return nil
			}
			if dimension, err := model.ResolveDimension(ref); err == nil {
				fieldID, err := assetID("field", report.SemanticModel+"."+dimension.Field)
				if err != nil {
					return err
				}
				edge(fromID, fieldID, edgeType)
				return addTableUse(dimension.Table)
			}
			measure, err := model.ResolveMeasure(ref)
			if err != nil {
				return err
			}
			measureID, err := assetID("measure", report.SemanticModel+"."+measure.Name)
			if err != nil {
				return err
			}
			edge(fromID, measureID, "uses_measure")
			return addTableUse(measure.Table)
		}
		for _, visual := range report.Visuals {
			for _, measureRef := range visual.Query.Measures {
				if err := addMeasureUse(measureRef); err != nil {
					return nil, nil, err
				}
			}
		}
		for _, table := range report.Tables {
			for _, column := range table.DataColumns {
				if err := addFieldUse(reportID, column.Field, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
			for _, measureRef := range table.Query.Measures {
				if err := addMeasureUse(measureRef); err != nil {
					return nil, nil, err
				}
			}
		}
		for _, page := range report.Pages {
			pageID, err := add("page", report.ID+"."+page.ID, reportID, page.Title, page.Description, page)
			if err != nil {
				return nil, nil, err
			}
			edge(reportID, pageID, "contains")
		}
		for filterName, filter := range report.Filters {
			filterID, err := add("filter", report.ID+"."+filterName, reportID, filter.Label, "", filter)
			if err != nil {
				return nil, nil, err
			}
			if err := addFieldUse(filterID, filter.Dimension, "filters_field"); err != nil {
				return nil, nil, err
			}
		}
		for visualName, visual := range report.Visuals {
			visualID, err := add("visual", report.ID+"."+visualName, reportID, visual.Title, "", visual)
			if err != nil {
				return nil, nil, err
			}
			for _, measure := range visual.Query.Measures {
				if measure.Measure.Expression != "" || measure.Measure.Expr != "" {
					if err := addTableUse(measure.Measure.Table); err != nil {
						return nil, nil, err
					}
					continue
				}
				resolved, err := model.ResolveMeasure(measure.Field)
				if err != nil {
					return nil, nil, err
				}
				measureID, err := assetID("measure", report.SemanticModel+"."+resolved.Name)
				if err != nil {
					return nil, nil, err
				}
				edge(visualID, measureID, "uses_measure")
			}
			for _, dimension := range visual.Query.Dimensions {
				if err := addFieldUse(visualID, dimension.Field, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
			if !visual.Query.Series.IsZero() {
				if err := addFieldUse(visualID, visual.Query.Series.Field, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
		}
		for tableName, table := range report.Tables {
			tableID, err := add("table", report.ID+"."+tableName, reportID, table.Title, "", table)
			if err != nil {
				return nil, nil, err
			}
			for _, column := range table.DataColumns {
				if err := addFieldUse(tableID, column.Field, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
			for _, row := range table.Rows {
				if err := addFieldUse(tableID, row, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
			for _, dimension := range table.ColumnDims {
				if err := addFieldUse(tableID, dimension, "uses_field"); err != nil {
					return nil, nil, err
				}
			}
			for _, measure := range table.Measures {
				resolved, err := model.ResolveMeasure(measure)
				if err != nil {
					return nil, nil, err
				}
				measureID, err := assetID("measure", report.SemanticModel+"."+resolved.Name)
				if err != nil {
					return nil, nil, err
				}
				edge(tableID, measureID, "uses_measure")
			}
		}
	}
	return assets, edges, nil
}

func connectionDescription(connection semantic.Connection) string {
	if connection.Kind == "" {
		return "connection"
	}
	return connection.Kind + " connection"
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
