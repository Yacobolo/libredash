package app

import (
	"net/http"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	workspacesearch "github.com/Yacobolo/libredash/internal/workspace/search"
	"github.com/go-chi/chi/v5"
)

func (s *Server) searchWorkspace(w http.ResponseWriter, r *http.Request) {
	types, err := workspacesearch.ParseTypes(r.URL.Query().Get("types"))
	if err != nil {
		writeJSONError(w, err, http.StatusBadRequest)
		return
	}
	workspaceID := s.workspaceID(chi.URLParam(r, "workspace"))
	results, err := s.workspaceSearchResults(r, workspaceID, r.URL.Query().Get("q"), types)
	if err != nil {
		writeJSONError(w, err, statusForNotFound(err))
		return
	}
	items, nextCursor, ok := pageSliceForRequest(w, r, results)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.SearchResponse{Items: items, Page: api.PageInfo{NextCursor: nextCursor}})
}

func (s *Server) workspaceSearchResults(r *http.Request, workspaceID, query string, types workspacesearch.TypeSet) ([]api.SearchResult, error) {
	documents := make([]workspacesearch.Document, 0)
	if metrics, ok := s.metricsForWorkspace(workspaceID); ok && metrics != nil {
		documents = append(documents, workspaceSearchDocuments(workspaceID, metrics)...)
	}
	assets, _, err := s.workspaceAssetsAndEdges(r, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		name := firstNonEmpty(asset.Title, asset.Key, asset.ID)
		documents = append(documents, workspacesearch.Document{
			ID:          asset.ID,
			Type:        "asset",
			Name:        name,
			Description: firstNonEmpty(asset.Description, asset.Type+" asset "+asset.Key),
			Refs: workspacesearch.Refs{
				AssetID: asset.ID,
			},
			Terms:  []string{asset.ID, asset.Type, asset.Key, asset.Title, asset.Description},
			Weight: -10,
		})
	}
	return searchResultsFromWorkspaceResults(workspacesearch.Rank(documents, workspacesearch.Query{Text: query, Types: types})), nil
}

func workspaceSearchDocuments(workspaceID string, metrics QueryMetrics) []workspacesearch.Document {
	catalog := metrics.Catalog()
	documents := make([]workspacesearch.Document, 0)
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
		documents = append(documents, workspacesearch.Document{
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

func dashboardSearchDocuments(report reportdef.Dashboard, model *semanticmodel.Model, pages []dashboard.Page) []workspacesearch.Document {
	out := []workspacesearch.Document{{
		ID:          report.ID,
		Type:        "dashboard",
		Name:        firstNonEmpty(report.Title, report.ID),
		Description: firstNonEmpty(report.Description, "Dashboard "+report.ID),
		Refs: workspacesearch.Refs{
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
		out = append(out, workspacesearch.Document{
			ID:          report.ID + "." + page.ID,
			Type:        "page",
			Name:        firstNonEmpty(page.Title, page.ID),
			Description: firstNonEmpty(page.Description, "Page "+page.ID+" in "+firstNonEmpty(report.Title, report.ID)),
			Refs: workspacesearch.Refs{
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

func dashboardComponentSearchDocument(report reportdef.Dashboard, page dashboard.Page, component dashboard.PageVisual) workspacesearch.Document {
	switch {
	case component.Visual != "":
		visual := report.Visuals[component.Visual]
		name := firstNonEmpty(component.Title, visual.Title, component.Visual)
		description := firstNonEmpty(component.Description, visual.Description, "Visual "+component.Visual+" on "+firstNonEmpty(page.Title, page.ID))
		return workspacesearch.Document{
			ID:          "visual:" + report.ID + "." + page.ID + "." + component.Visual,
			Type:        "visual",
			Name:        name,
			Description: description,
			Refs: workspacesearch.Refs{
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
		return workspacesearch.Document{
			ID:          "table:" + report.ID + "." + page.ID + "." + component.Table,
			Type:        "table",
			Name:        name,
			Description: description,
			Refs: workspacesearch.Refs{
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
		return workspacesearch.Document{
			ID:          "filter:" + report.ID + "." + page.ID + "." + component.Filter,
			Type:        "filter",
			Name:        name,
			Description: description,
			Refs: workspacesearch.Refs{
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
		return workspacesearch.Document{
			ID:          report.ID + "." + page.ID + "." + component.ID,
			Type:        "page",
			Name:        name,
			Description: firstNonEmpty(component.Description, component.Kind+" component on "+firstNonEmpty(page.Title, page.ID)),
			Refs: workspacesearch.Refs{
				DashboardID: report.ID,
				PageID:      page.ID,
			},
			Terms: []string{report.ID, report.Title, page.ID, page.Title, component.ID, component.Kind, component.Title, component.Description},
		}
	}
}

func semanticModelSearchDocuments(modelID, title, description string, model *semanticmodel.Model) []workspacesearch.Document {
	out := []workspacesearch.Document{{
		ID:          modelID,
		Type:        "semantic_model",
		Name:        firstNonEmpty(title, modelID),
		Description: firstNonEmpty(description, "Semantic model "+modelID),
		Refs: workspacesearch.Refs{
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
		out = append(out, workspacesearch.Document{
			ID:          modelID + "." + sourceID,
			Type:        "source",
			Name:        sourceID,
			Description: firstNonEmpty(source.Description, "Source "+sourceID),
			Refs: workspacesearch.Refs{
				ModelID: modelID,
			},
			Terms:  []string{modelID, model.Title, sourceID, source.Description, source.Format, source.Connection, source.Object, source.Path},
			Weight: 10,
		})
	}
	for _, datasetID := range sortedMapKeys(model.Tables) {
		table := model.Tables[datasetID]
		out = append(out, workspacesearch.Document{
			ID:          modelID + "." + datasetID,
			Type:        "dataset",
			Name:        datasetID,
			Description: firstNonEmpty(table.Description, "Dataset "+datasetID),
			Refs: workspacesearch.Refs{
				ModelID:   modelID,
				DatasetID: datasetID,
			},
			Terms:  []string{modelID, model.Title, datasetID, table.Kind, table.Source, strings.Join(table.Sources, " "), table.Description, table.PrimaryKey, table.Grain},
			Weight: 15,
		})
		for _, field := range semanticDatasetFields(model, datasetID, table) {
			typ := "field"
			if field.Kind == "measure" {
				typ = "measure"
			}
			out = append(out, workspacesearch.Document{
				ID:          modelID + "." + field.ID,
				Type:        typ,
				Name:        firstNonEmpty(field.Label, field.Name, field.ID),
				Description: firstNonEmpty(field.Description, typ+" "+field.ID),
				Refs: workspacesearch.Refs{
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

func searchResultsFromWorkspaceResults(results []workspacesearch.Result) []api.SearchResult {
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
