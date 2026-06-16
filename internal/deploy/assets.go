package deploy

import (
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
		for sourceName, source := range model.Sources {
			id, err := add("source", modelEntry.ID+"."+sourceName, modelID, sourceName, source.File, source)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
		}
		for cacheName, cache := range model.Cache.Tables {
			id, err := add("cache_table", modelEntry.ID+"."+cacheName, modelID, cacheName, cache.Description, cache)
			if err != nil {
				return nil, nil, err
			}
			edge(modelID, id, "contains")
			for sourceName := range model.Sources {
				edge(id, byKey["source:"+modelEntry.ID+"."+sourceName], "reads_source")
			}
		}
		for datasetName, dataset := range model.Datasets {
			datasetID, err := add("dataset", modelEntry.ID+"."+datasetName, modelID, datasetName, "", dataset)
			if err != nil {
				return nil, nil, err
			}
			edge(datasetID, byKey["cache_table:"+modelEntry.ID+"."+dataset.Source], "uses_cache_table")
			for dimensionName, dimension := range dataset.Dimensions {
				id, err := add("dimension", modelEntry.ID+"."+datasetName+"."+dimensionName, datasetID, dimensionLabel(dimensionName, dimension.Label), "", dimension)
				if err != nil {
					return nil, nil, err
				}
				edge(datasetID, id, "contains")
			}
			for measureName, measure := range dataset.Measures {
				id, err := add("measure", modelEntry.ID+"."+datasetName+"."+measureName, datasetID, measureLabel(measureName, measure.Label), "", measure)
				if err != nil {
					return nil, nil, err
				}
				edge(datasetID, id, "contains")
			}
		}
	}
	for _, reportEntry := range workspace.Catalog.Dashboards {
		report := workspace.Dashboards[reportEntry.ID]
		reportID, err := add("dashboard", reportEntry.ID, catalogID, reportEntry.Title, reportEntry.Description, report)
		if err != nil {
			return nil, nil, err
		}
		edge(reportID, byKey["semantic_model:"+reportEntry.SemanticModel], "uses_model")
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
			edge(filterID, byKey["dimension:"+report.SemanticModel+"."+filter.Dataset+"."+filter.Dimension], "filters_dimension")
		}
		for kpiName, kpi := range report.KPIs {
			kpiID, err := add("kpi", report.ID+"."+kpiName, reportID, kpi.Title, kpi.Note, kpi)
			if err != nil {
				return nil, nil, err
			}
			edge(kpiID, byKey["measure:"+report.SemanticModel+"."+kpi.Dataset+"."+kpi.Measure], "uses_measure")
		}
		for visualName, visual := range report.Visuals {
			visualID, err := add("visual", report.ID+"."+visualName, reportID, visual.Title, "", visual)
			if err != nil {
				return nil, nil, err
			}
			edge(visualID, byKey["dataset:"+report.SemanticModel+"."+visual.Dataset], "uses_dataset")
			for _, measure := range visual.Query.Measures {
				edge(visualID, byKey["measure:"+report.SemanticModel+"."+visual.Dataset+"."+measure], "uses_measure")
			}
			for _, dimension := range visual.Query.Dimensions {
				edge(visualID, byKey["dimension:"+report.SemanticModel+"."+visual.Dataset+"."+dimension], "uses_dimension")
			}
			if visual.Query.Series != "" {
				edge(visualID, byKey["dimension:"+report.SemanticModel+"."+visual.Dataset+"."+visual.Query.Series], "uses_dimension")
			}
		}
		for tableName, table := range report.Tables {
			tableID, err := add("table", report.ID+"."+tableName, reportID, table.Title, "", table)
			if err != nil {
				return nil, nil, err
			}
			edge(tableID, byKey["dataset:"+report.SemanticModel+"."+table.Dataset], "uses_dataset")
		}
	}
	return assets, edges, nil
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
