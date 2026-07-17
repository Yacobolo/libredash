package http

import (
	nethttp "net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/workspace/search"
	"github.com/go-chi/chi/v5"
)

func (h Handler) SearchWorkspace(w nethttp.ResponseWriter, r *nethttp.Request) {
	types, err := search.ParseTypes(r.URL.Query().Get("types"))
	if err != nil {
		writeJSONError(w, err, nethttp.StatusBadRequest)
		return
	}
	workspaceID := h.workspaceID(chi.URLParam(r, "workspace"))
	results, err := h.workspaceSearchResults(r, workspaceID, r.URL.Query().Get("q"), types)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	results, err = h.filterReadableSearchResults(r, workspaceID, results)
	if err != nil {
		writeJSONError(w, err, nethttp.StatusInternalServerError)
		return
	}
	items, nextCursor, ok := pageSliceForRequest(w, r, results)
	if !ok {
		return
	}
	writeJSON(w, nethttp.StatusOK, api.SearchResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (h Handler) filterReadableSearchResults(r *nethttp.Request, workspaceID string, rows []api.SearchResult) ([]api.SearchResult, error) {
	out := make([]api.SearchResult, 0, len(rows))
	for _, row := range rows {
		object, ok := searchResultObject(workspaceID, row)
		if !ok {
			out = append(out, row)
			continue
		}
		allowed, err := h.ReadModel.CanReadObject(r, object)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, row)
		}
	}
	return out, nil
}

func searchResultObject(workspaceID string, row api.SearchResult) (access.ObjectRef, bool) {
	workspaceObject := access.WorkspaceObject(workspaceID)
	if row.AssetID != "" {
		return assetObjectForID(workspaceID, row.AssetID)
	}
	if row.DashboardID != "" {
		return access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, row.DashboardID, workspaceObject), true
	}
	if row.ModelID == "" {
		return access.ObjectRef{}, false
	}
	model := access.ItemObjectWithParent(access.SecurableSemanticModel, workspaceID, row.ModelID, workspaceObject)
	if row.DatasetID == "" {
		return model, true
	}
	dataset := access.ItemObjectWithParent(access.SecurableDataset, workspaceID, row.ModelID+"/"+row.DatasetID, model)
	if row.FieldID == "" {
		return dataset, true
	}
	return access.ItemObjectWithParent(access.SecurableColumn, workspaceID, row.ModelID+"/"+row.DatasetID+"/"+row.FieldID, dataset), true
}

func (h Handler) workspaceSearchResults(r *nethttp.Request, workspaceID, query string, types search.TypeSet) ([]api.SearchResult, error) {
	documents := make([]search.Document, 0)
	if metrics, ok := h.metricsForWorkspace(workspaceID); ok && metrics != nil {
		documents = append(documents, workspaceSearchDocuments(workspaceID, metrics)...)
	}
	assets, _, err := h.assetsAndEdges(r, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		name := firstNonEmpty(asset.Title, asset.Key, asset.ID)
		documents = append(documents, search.Document{
			ID:          asset.ID,
			Type:        "asset",
			Name:        name,
			Description: firstNonEmpty(asset.Description, asset.Type+" asset "+asset.Key),
			Refs: search.Refs{
				AssetID: asset.ID,
			},
			Terms:  []string{asset.ID, asset.Type, asset.Key, asset.Title, asset.Description},
			Weight: -10,
		})
	}
	return searchResultsFromWorkspaceResults(search.Rank(documents, search.Query{Text: query, Types: types})), nil
}

func workspaceSearchDocuments(workspaceID string, metrics Metrics) []search.Document {
	catalog := metrics.Catalog()
	documents := make([]search.Document, 0)
	for _, dashboardSummary := range catalog.Dashboards {
		report, model, ok := metrics.Report(dashboardSummary.ID)
		if !ok {
			continue
		}
		documents = append(documents, dashboardSearchDocuments(report, model, metrics.Pages(report.ID))...)
	}
	for _, modelSummary := range catalog.Models {
		model := semanticModelForID(metrics, modelSummary.ID)
		documents = append(documents, semanticModelSearchDocuments(modelSummary.ID, modelSummary.Title, modelSummary.Description, model)...)
	}
	if workspaceID != "" {
		documents = append(documents, search.Document{
			ID:          workspaceID,
			Type:        "asset",
			Name:        workspaceID,
			Description: "Workspace " + workspaceID,
			Terms:       []string{workspaceID, "workspace"},
			Weight:      -20,
		})
	}
	return documents
}

func dashboardSearchDocuments(report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page) []search.Document {
	out := []search.Document{{
		ID:          report.ID,
		Type:        "dashboard",
		Name:        firstNonEmpty(report.Title, report.ID),
		Description: firstNonEmpty(report.Description, "Dashboard "+report.ID),
		Refs: search.Refs{
			DashboardID: report.ID,
			ModelID:     report.SemanticModel,
		},
		Terms:  append([]string{report.ID, report.Title, report.Description, report.SemanticModel}, dashboardDeepTerms(report)...),
		Weight: 20,
	}}
	if len(pages) == 0 {
		pages = report.Pages
	}
	for _, page := range pages {
		page = page.WithDefaults()
		out = append(out, search.Document{
			ID:          report.ID + "." + page.ID,
			Type:        "page",
			Name:        firstNonEmpty(page.Title, page.ID),
			Description: firstNonEmpty(page.Description, "Page "+page.ID+" in "+firstNonEmpty(report.Title, report.ID)),
			Refs: search.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
				ModelID:     report.SemanticModel,
			},
			Terms:  []string{report.ID, report.Title, page.ID, page.Title, page.Description, report.SemanticModel},
			Weight: 10,
		})
		for _, component := range page.PlacedVisuals() {
			out = append(out, dashboardComponentSearchDocument(report, page, component))
		}
	}
	if model != nil && report.SemanticModel == "" {
		for i := range out {
			out[i].Refs.ModelID = model.Name
			out[i].Terms = append(out[i].Terms, model.Name, model.Title)
		}
	}
	return out
}

func dashboardDeepTerms(report reportdef.Dashboard) []string {
	terms := make([]string, 0)
	for id, filter := range report.Filters {
		terms = append(terms, id, filter.Label, filter.Description, filter.Dimension, filter.Type, filter.URLParam)
	}
	for id, visual := range report.Visuals {
		terms = append(terms, id, visual.Title, visual.Description, visual.Type, visual.Kind, visual.Shape, visual.Query.Time.Field)
		terms = append(terms, fieldRefTerms(visual.Query.Dimensions)...)
		terms = append(terms, fieldRefTerms(visual.Query.Measures)...)
	}
	for id, table := range report.Tables {
		terms = append(terms, id, table.Title, table.Description, table.Query.Table)
		terms = append(terms, table.Query.Fields...)
		terms = append(terms, fieldRefTerms(table.Query.Columns)...)
		terms = append(terms, fieldRefTerms(table.Query.Rows)...)
		terms = append(terms, fieldRefTerms(table.Query.Measures)...)
	}
	for _, page := range report.Pages {
		terms = append(terms, page.ID, page.Title, page.Description)
		for _, component := range page.Visuals {
			terms = append(terms, component.ID, component.Kind, component.Title, component.Description, component.Visual, component.Table, component.Filter)
		}
	}
	return terms
}

func dashboardComponentSearchDocument(report reportdef.Dashboard, page dashboard.Page, component dashboard.PageVisual) search.Document {
	switch {
	case component.Visual != "":
		visual := report.Visuals[component.Visual]
		name := firstNonEmpty(component.Title, visual.Title, component.Visual)
		description := firstNonEmpty(component.Description, visual.Description, "Visual "+component.Visual+" on "+firstNonEmpty(page.Title, page.ID))
		return search.Document{
			ID:          "visual:" + report.ID + "." + page.ID + "." + component.Visual,
			Type:        "visual",
			Name:        name,
			Description: description,
			Refs: search.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
				VisualID:    component.Visual,
				ModelID:     report.SemanticModel,
			},
			Terms: []string{
				report.ID, report.Title, page.ID, page.Title, component.ID, component.Kind,
				component.Visual, component.Title, component.Description, visual.Title, visual.Description,
				visual.Type, visual.Kind, visual.Shape, report.SemanticModel, strings.Join(fieldRefTerms(visual.Query.Dimensions), " "),
				strings.Join(fieldRefTerms(visual.Query.Measures), " "), visual.Query.Time.Field,
			},
			Weight: 30,
		}
	case component.Table != "":
		table := report.Tables[component.Table]
		name := firstNonEmpty(component.Title, table.Title, component.Table)
		description := firstNonEmpty(component.Description, table.Description, "Table "+component.Table+" on "+firstNonEmpty(page.Title, page.ID))
		return search.Document{
			ID:          "table:" + report.ID + "." + page.ID + "." + component.Table,
			Type:        "table",
			Name:        name,
			Description: description,
			Refs: search.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
				TableID:     component.Table,
				ModelID:     report.SemanticModel,
				DatasetID:   table.Query.Table,
			},
			Terms: []string{
				report.ID, report.Title, page.ID, page.Title, component.ID, component.Kind,
				component.Table, component.Title, component.Description, table.Title, table.Description,
				table.Query.Table, strings.Join(table.Query.Fields, " "), strings.Join(fieldRefTerms(table.Query.Columns), " "),
				strings.Join(fieldRefTerms(table.Query.Rows), " "), strings.Join(fieldRefTerms(table.Query.Measures), " "),
			},
			Weight: 25,
		}
	case component.Filter != "":
		filter := report.Filters[component.Filter]
		name := firstNonEmpty(component.Title, filter.Label, component.Filter)
		description := firstNonEmpty(component.Description, filter.Description, "Filter "+component.Filter+" on "+firstNonEmpty(page.Title, page.ID))
		return search.Document{
			ID:          "filter:" + report.ID + "." + page.ID + "." + component.Filter,
			Type:        "filter",
			Name:        name,
			Description: description,
			Refs: search.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
				FilterID:    component.Filter,
				ModelID:     report.SemanticModel,
				FieldID:     filter.Dimension,
			},
			Terms: []string{
				report.ID, report.Title, page.ID, page.Title, component.ID, component.Kind,
				component.Filter, component.Title, component.Description, filter.Label, filter.Description,
				filter.Dimension, filter.Type, filter.URLParam,
			},
			Weight: 20,
		}
	default:
		name := firstNonEmpty(component.Title, component.ID)
		return search.Document{
			ID:          report.ID + "." + page.ID + "." + component.ID,
			Type:        "page",
			Name:        name,
			Description: firstNonEmpty(component.Description, component.Kind+" component on "+firstNonEmpty(page.Title, page.ID)),
			Refs: search.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
			},
			Terms: []string{report.ID, report.Title, page.ID, page.Title, component.ID, component.Kind, component.Title, component.Description},
		}
	}
}

func semanticModelSearchDocuments(modelID, title, description string, model *semanticmodel.Model) []search.Document {
	out := []search.Document{{
		ID:          modelID,
		Type:        "semantic_model",
		Name:        firstNonEmpty(title, modelID),
		Description: firstNonEmpty(description, "Semantic model "+modelID),
		Refs: search.Refs{
			ModelID: modelID,
		},
		Terms:  []string{modelID, title, description},
		Weight: 20,
	}}
	if model == nil {
		return out
	}
	for _, sourceID := range sortedMapKeys(model.Sources) {
		source := model.Sources[sourceID]
		out = append(out, search.Document{
			ID:          modelID + "." + sourceID,
			Type:        "source",
			Name:        sourceID,
			Description: firstNonEmpty(source.Description, "Source "+sourceID),
			Refs: search.Refs{
				ModelID: modelID,
			},
			Terms:  []string{modelID, model.Title, sourceID, source.Description, source.Format, source.Connection, source.Object, source.Path},
			Weight: 10,
		})
	}
	for _, datasetID := range sortedMapKeys(model.Tables) {
		table := model.Tables[datasetID]
		out = append(out, search.Document{
			ID:          modelID + "." + datasetID,
			Type:        "semantic_table",
			Name:        datasetID,
			Description: firstNonEmpty(table.Description, "Semantic table "+datasetID),
			Refs: search.Refs{
				ModelID:   modelID,
				DatasetID: datasetID,
			},
			Terms:  []string{modelID, model.Title, datasetID, table.Source, strings.Join(table.Sources, " "), table.Description, table.PrimaryKey, table.Grain},
			Weight: 15,
		})
		for _, field := range semanticDatasetFields(model, datasetID, table) {
			typ := "field"
			if field.Kind == "measure" {
				typ = "measure"
			}
			out = append(out, search.Document{
				ID:          modelID + "." + field.ID,
				Type:        typ,
				Name:        firstNonEmpty(field.Label, field.Name, field.ID),
				Description: firstNonEmpty(field.Description, typ+" "+field.ID),
				Refs: search.Refs{
					ModelID:   modelID,
					DatasetID: datasetID,
					FieldID:   field.ID,
				},
				Terms:  []string{modelID, model.Title, datasetID, field.ID, field.Name, field.Label, field.Description, field.Table, field.Unit, field.Format, field.Grain, field.Time, strings.Join(field.Grains, " ")},
				Weight: 25,
			})
		}
	}
	return out
}

func fieldRefTerms(fields []reportdef.FieldRef) []string {
	out := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		out = append(out, field.Field, field.Alias)
	}
	return out
}

func semanticDatasetMeasureCount(model *semanticmodel.Model, datasetID string) int {
	if model == nil {
		return 0
	}
	count := 0
	for _, measure := range model.Measures {
		if measure.Fact == datasetID {
			count++
		}
	}
	return count
}

func semanticDatasetFields(model *semanticmodel.Model, datasetID string, table semanticmodel.Table) []api.SemanticFieldResponse {
	out := make([]api.SemanticFieldResponse, 0, len(table.Dimensions)+semanticDatasetMeasureCount(model, datasetID))
	for _, fieldID := range sortedMapKeys(table.Dimensions) {
		dimension := table.Dimensions[fieldID]
		out = append(out, api.SemanticFieldResponse{
			ID:          datasetID + "." + fieldID,
			Kind:        "dimension",
			Table:       datasetID,
			Name:        fieldID,
			Label:       dimension.Label,
			Description: dimension.Description,
		})
	}
	for _, measureID := range sortedMapKeys(model.Measures) {
		measure := model.Measures[measureID]
		if measure.Fact != datasetID {
			continue
		}
		out = append(out, semanticMeasureFieldDTO(measureID, datasetID, measureID, measure))
	}
	return out
}

func semanticMeasureFieldDTO(id, datasetID, name string, measure semanticmodel.MetricMeasure) api.SemanticFieldResponse {
	return api.SemanticFieldResponse{
		ID:          id,
		Kind:        "measure",
		Table:       datasetID,
		Name:        name,
		Label:       measure.Label,
		Description: measure.Description,
		Unit:        measure.Unit,
		Format:      measure.Format,
	}
}

func sortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func semanticModelForID(metrics Metrics, modelID string) *semanticmodel.Model {
	if metrics == nil {
		return nil
	}
	if model, ok := metrics.SemanticModel(modelID); ok {
		return model
	}
	for _, row := range metrics.Catalog().Models {
		if row.ID != modelID {
			continue
		}
		if model, ok := metrics.SemanticModel(row.ID); ok {
			return model
		}
	}
	return nil
}

func searchResultsFromWorkspaceResults(results []search.Result) []api.SearchResult {
	out := make([]api.SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, api.SearchResult{
			ID:          result.ID,
			Type:        result.Type,
			Name:        result.Name,
			Description: result.Description,
			DashboardID: result.DashboardID,
			PageID:      result.PageID,
			VisualID:    result.VisualID,
			TableID:     result.TableID,
			FilterID:    result.FilterID,
			ModelID:     result.ModelID,
			DatasetID:   result.DatasetID,
			FieldID:     result.FieldID,
			AssetID:     result.AssetID,
		})
	}
	return out
}
