package deploy

import (
	"regexp"
	"sort"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func ExtractAssets(workspaceID, deploymentID string, workspace *semantic.Workspace) ([]platform.Asset, []platform.AssetEdge, error) {
	assets := []platform.Asset{}
	edges := []platform.AssetEdge{}
	byKey := map[string]string{}
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
		edges = append(edges, platform.NewAssetEdge(workspaceID, deploymentID, fromID, toID, typ))
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
			edge(id, byKey["connection:"+modelEntry.ID+"."+source.Connection], "uses_connection")
		}
		for tableName, table := range model.Tables {
			id, err := add("model_table", modelEntry.ID+"."+tableName, modelID, tableName, table.Description, table)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			if table.Source != "" {
				edge(id, byKey["source:"+modelEntry.ID+"."+table.Source], "reads_source")
			} else {
				for _, sourceName := range transformSourceRefs(table.Transform.SQL, model.Sources) {
					edge(id, byKey["source:"+modelEntry.ID+"."+sourceName], "reads_source")
				}
			}
		}
	}
	for _, viewEntry := range workspace.Catalog.MetricViews {
		view := workspace.MetricViews[viewEntry.ID]
		viewID, err := add("metric_view", viewEntry.ID, byKey["semantic_model:"+view.SemanticModel], viewEntry.Title, viewEntry.Description, view)
		if err != nil {
			return nil, nil, err
		}
		edge(byKey["semantic_model:"+view.SemanticModel], viewID, "contains")
		edge(viewID, byKey["model_table:"+view.SemanticModel+"."+view.BaseTable], "uses_model_table")
		for dimensionName, dimension := range view.Dimensions {
			id, err := add("dimension", view.ID+"."+dimensionName, viewID, dimensionLabel(dimensionName, dimension.Label), "", dimension)
			if err != nil {
				return nil, nil, err
			}
			edge(viewID, id, "contains")
		}
		for measureName, measure := range view.Measures {
			id, err := add("measure", view.ID+"."+measureName, viewID, measureLabel(measureName, measure.Label), "", measure)
			if err != nil {
				return nil, nil, err
			}
			edge(viewID, id, "contains")
		}
	}
	for _, reportEntry := range workspace.Catalog.Dashboards {
		report := workspace.Dashboards[reportEntry.ID]
		reportID, err := add("dashboard", reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, report)
		if err != nil {
			return nil, nil, err
		}
		for _, viewID := range report.MetricViews {
			edge(reportID, byKey["metric_view:"+viewID], "uses_metric_view")
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
			edge(filterID, byKey["dimension:"+filter.MetricView+"."+filter.Dimension], "filters_dimension")
		}
		for visualName, visual := range report.Visuals {
			visualID, err := add("visual", report.ID+"."+visualName, reportID, visual.Title, "", visual)
			if err != nil {
				return nil, nil, err
			}
			edge(visualID, byKey["metric_view:"+visual.MetricView], "uses_metric_view")
			for _, measure := range visual.Query.Measures {
				edge(visualID, byKey["measure:"+visual.MetricView+"."+measure.Field], "uses_measure")
			}
			for _, dimension := range visual.Query.Dimensions {
				edge(visualID, byKey["dimension:"+visual.MetricView+"."+dimension.Field], "uses_dimension")
			}
			if !visual.Query.Series.IsZero() {
				edge(visualID, byKey["dimension:"+visual.MetricView+"."+visual.Query.Series.Field], "uses_dimension")
			}
		}
		for tableName, table := range report.Tables {
			tableID, err := add("table", report.ID+"."+tableName, reportID, table.Title, "", table)
			if err != nil {
				return nil, nil, err
			}
			edge(tableID, byKey["metric_view:"+table.MetricView], "uses_metric_view")
			for _, row := range table.Rows {
				edge(tableID, byKey["dimension:"+table.MetricView+"."+row], "uses_dimension")
			}
			for _, dimension := range table.ColumnDims {
				edge(tableID, byKey["dimension:"+table.MetricView+"."+dimension], "uses_dimension")
			}
			for _, measure := range table.Measures {
				edge(tableID, byKey["measure:"+table.MetricView+"."+measure], "uses_measure")
			}
		}
	}
	return assets, edges, nil
}

func transformSourceRefs(sql string, sources map[string]semantic.Source) []string {
	if sql == "" || len(sources) == 0 {
		return nil
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, name := range names {
		pattern := regexp.MustCompile(`(?i)\braw\.` + regexp.QuoteMeta(name) + `\b`)
		if pattern.MatchString(sql) {
			out = append(out, name)
		}
	}
	return out
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
