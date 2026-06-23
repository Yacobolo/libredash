package agentapp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
	"github.com/Yacobolo/libredash/pkg/agent"
)

const maxAgentRows = 50

type Metrics interface {
	Catalog() dashboard.Catalog
	Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool)
	Pages(dashboardID string) []dashboard.Page
	DefaultFilters(dashboardID string) dashboard.Filters
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
}

func (s *Service) toolDefinitions(scope Scope) []agent.ToolDefinition {
	return []agent.ToolDefinition{
		{
			Name:        "list_dashboards",
			Description: "List dashboards available in the current workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, _ json.RawMessage) (any, error) {
				return dashboardListPayload{Dashboards: s.metrics.Catalog().Dashboards}, nil
			}),
		},
		{
			Name:        "describe_dashboard",
			Description: "Return a compact dashboard manifest with page/component references. Use query_dashboard_page or describe_metric_view for details.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"dashboard_id":{"type":"string"}},"required":["dashboard_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					DashboardID string `json:"dashboard_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				report, model, ok := s.metrics.Report(input.DashboardID)
				if !ok {
					return nil, fmt.Errorf("dashboard %q not found", input.DashboardID)
				}
				return dashboardManifest(report, model, s.metrics.Pages(input.DashboardID)), nil
			}),
		},
		{
			Name:        "list_metric_views",
			Description: "List metric views available in the current workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, _ json.RawMessage) (any, error) {
				return metricViewListPayload{MetricViews: s.metrics.Catalog().MetricViews}, nil
			}),
		},
		{
			Name:        "describe_metric_view",
			Description: "Describe a metric view catalog entry and where dashboards use it.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"metric_view_id":{"type":"string"}},"required":["metric_view_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					MetricViewID string `json:"metric_view_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				view, ok := metricViewSummary(s.metrics.Catalog(), input.MetricViewID)
				if !ok {
					return nil, fmt.Errorf("metric view %q not found", input.MetricViewID)
				}
				return metricViewDescriptionPayload{
					MetricView: view,
					Usage:      metricViewUsage(s.metrics, input.MetricViewID),
					DetailTools: map[string]string{
						"dashboard": "describe_dashboard",
						"page_data": "query_dashboard_page",
						"table":     "query_table",
					},
				}, nil
			}),
		},
		{
			Name:        "describe_model",
			Description: "Describe a semantic model summary, its model tables, metric views, and dashboard usage.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"model_id":{"type":"string"}},"required":["model_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					ModelID string `json:"model_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				model, ok := modelDescription(s.metrics, input.ModelID)
				if !ok {
					return nil, fmt.Errorf("model %q not found", input.ModelID)
				}
				return model, nil
			}),
		},
		{
			Name:        "query_dashboard_page",
			Description: "Return a bounded data snapshot for a dashboard page using optional dashboard filters.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"dashboard_id":{"type":"string"},"page_id":{"type":"string"},"filters":{"type":"object"}},"required":["dashboard_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					DashboardID string            `json:"dashboard_id"`
					PageID      string            `json:"page_id"`
					Filters     dashboard.Filters `json:"filters"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				filters := input.Filters
				if filters.Controls == nil && filters.VisualSelections == nil {
					filters = s.metrics.DefaultFilters(input.DashboardID)
				}
				patch, err := s.metrics.QueryDashboardPage(ctx, input.DashboardID, input.PageID, filters)
				if err != nil {
					return nil, err
				}
				return boundedPatch(patch), nil
			}),
		},
		{
			Name:        "query_table",
			Description: "Return a bounded row snapshot for a dashboard table. Count is capped at 50.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"dashboard_id":{"type":"string"},"page_id":{"type":"string"},"table_id":{"type":"string"},"count":{"type":"integer","minimum":1},"filters":{"type":"object"}},"required":["dashboard_id","table_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					DashboardID string            `json:"dashboard_id"`
					PageID      string            `json:"page_id"`
					TableID     string            `json:"table_id"`
					Count       int               `json:"count"`
					Filters     dashboard.Filters `json:"filters"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				count := input.Count
				if count <= 0 || count > maxAgentRows {
					count = maxAgentRows
				}
				filters := input.Filters
				if filters.Controls == nil && filters.VisualSelections == nil {
					filters = s.metrics.DefaultFilters(input.DashboardID)
				}
				request := s.metrics.NormalizeTableRequest(input.DashboardID, dashboard.TableRequest{Table: input.TableID, Block: "a", Count: count})
				request.Count = count
				table, err := s.metrics.QueryTablePage(ctx, input.DashboardID, input.PageID, filters, request)
				if err != nil {
					return nil, err
				}
				return boundedTable(table), nil
			}),
		},
	}
}

func (s *Service) tool(fn func(ctx context.Context, raw json.RawMessage) (any, error)) agent.ToolHandler {
	return agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
		content, err := fn(ctx, call.Arguments)
		if err != nil {
			return agent.ToolResult{}, err
		}
		return agent.ToolResult{Content: content}, nil
	})
}

func boundedPatch(patch dashboard.Patch) dashboard.Patch {
	for key, visual := range patch.Visuals {
		if len(visual.Data) > maxAgentRows {
			visual.Data = visual.Data[:maxAgentRows]
		}
		patch.Visuals[key] = visual
	}
	return patch
}

func boundedTable(table dashboard.Table) dashboard.Table {
	for key, block := range table.Blocks {
		if len(block.Rows) > maxAgentRows {
			block.Rows = block.Rows[:maxAgentRows]
		}
		table.Blocks[key] = block
	}
	if table.AvailableRows > maxAgentRows {
		table.AvailableRows = maxAgentRows
	}
	return table
}

type dashboardListPayload struct {
	Dashboards []dashboard.CatalogDashboard `json:"dashboards"`
}

type metricViewListPayload struct {
	MetricViews []dashboard.CatalogMetricView `json:"metric_views"`
}

type metricViewDescriptionPayload struct {
	MetricView  dashboard.CatalogMetricView `json:"metric_view"`
	Usage       []metricViewUsageSummary    `json:"usage"`
	DetailTools map[string]string           `json:"detail_tools"`
}

type metricViewUsageSummary struct {
	DashboardID    string `json:"dashboard_id"`
	DashboardTitle string `json:"dashboard_title"`
	Visuals        int    `json:"visuals"`
	Tables         int    `json:"tables"`
	Filters        int    `json:"filters"`
	Pages          int    `json:"pages"`
}

type modelRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func modelSummary(model *semantic.Model) *modelRef {
	if model == nil {
		return nil
	}
	return &modelRef{ID: model.Name, Title: model.Title}
}

func metricViewSummary(catalog dashboard.Catalog, id string) (dashboard.CatalogMetricView, bool) {
	for _, view := range catalog.MetricViews {
		if view.ID == id {
			return view, true
		}
	}
	return dashboard.CatalogMetricView{}, false
}

func metricViewUsage(metrics Metrics, id string) []metricViewUsageSummary {
	catalog := metrics.Catalog()
	usage := make([]metricViewUsageSummary, 0)
	for _, dashboardSummary := range catalog.Dashboards {
		report, _, ok := metrics.Report(dashboardSummary.ID)
		if !ok || !containsString(report.MetricViews, id) {
			continue
		}
		visuals, tables, filters := 0, 0, 0
		for _, visual := range report.Visuals {
			if visual.MetricView == id || visual.Query.MetricView == id {
				visuals++
			}
		}
		for _, table := range report.Tables {
			if table.MetricView == id || table.Query.MetricView == id {
				tables++
			}
		}
		for _, filter := range report.Filters {
			if filter.MetricView == id {
				filters++
			}
		}
		usage = append(usage, metricViewUsageSummary{
			DashboardID:    report.ID,
			DashboardTitle: report.Title,
			Visuals:        visuals,
			Tables:         tables,
			Filters:        filters,
			Pages:          len(metrics.Pages(report.ID)),
		})
	}
	return usage
}

type modelDescriptionPayload struct {
	ID          string                        `json:"id"`
	Title       string                        `json:"title"`
	Description string                        `json:"description"`
	MetricViews []dashboard.CatalogMetricView `json:"metric_views"`
	Dashboards  []modelDashboardUsage         `json:"dashboards"`
	Counts      *semanticModelCounts          `json:"counts,omitempty"`
	Tables      []semanticModelTableSummary   `json:"tables,omitempty"`
}

type semanticModelCounts struct {
	Sources       int `json:"sources"`
	ModelTables   int `json:"model_tables"`
	Relationships int `json:"relationships"`
}

type semanticModelTableSummary struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Dimensions  int    `json:"dimensions"`
	Measures    int    `json:"measures"`
}

type modelDashboardUsage struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	MetricViews []string `json:"metric_views"`
	Pages       int      `json:"pages"`
}

func modelDescription(metrics Metrics, id string) (modelDescriptionPayload, bool) {
	catalog := metrics.Catalog()
	var catalogModel dashboard.CatalogModel
	for _, model := range catalog.Models {
		if model.ID == id {
			catalogModel = model
			break
		}
	}
	if catalogModel.ID == "" {
		return modelDescriptionPayload{}, false
	}

	out := modelDescriptionPayload{
		ID:          catalogModel.ID,
		Title:       catalogModel.Title,
		Description: catalogModel.Description,
		MetricViews: metricViewsForModel(catalog, id),
		Dashboards:  dashboardsForModel(metrics, id),
	}
	if model := semanticModelForID(metrics, id); model != nil {
		out.Counts = &semanticModelCounts{
			Sources:       len(model.Sources),
			ModelTables:   len(model.Tables),
			Relationships: len(model.Relationships),
		}
		tables := make([]semanticModelTableSummary, 0, len(model.Tables))
		for tableID, table := range model.Tables {
			tables = append(tables, semanticModelTableSummary{
				ID:          tableID,
				Kind:        table.Kind,
				Source:      table.Source,
				Description: table.Description,
				Dimensions:  len(table.Dimensions),
				Measures:    len(table.Measures),
			})
		}
		out.Tables = tables
	}
	return out, true
}

func metricViewsForModel(catalog dashboard.Catalog, modelID string) []dashboard.CatalogMetricView {
	out := make([]dashboard.CatalogMetricView, 0)
	for _, view := range catalog.MetricViews {
		if view.SemanticModel == modelID {
			out = append(out, view)
		}
	}
	return out
}

func dashboardsForModel(metrics Metrics, modelID string) []modelDashboardUsage {
	out := make([]modelDashboardUsage, 0)
	for _, dashboardSummary := range metrics.Catalog().Dashboards {
		report, model, ok := metrics.Report(dashboardSummary.ID)
		if !ok || model == nil || model.Name != modelID {
			continue
		}
		out = append(out, modelDashboardUsage{
			ID:          report.ID,
			Title:       report.Title,
			MetricViews: report.MetricViews,
			Pages:       len(metrics.Pages(report.ID)),
		})
	}
	return out
}

func semanticModelForID(metrics Metrics, modelID string) *semantic.Model {
	for _, dashboardSummary := range metrics.Catalog().Dashboards {
		_, model, ok := metrics.Report(dashboardSummary.ID)
		if ok && model != nil && model.Name == modelID {
			return model
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

type dashboardManifestSummary struct {
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description,omitempty"`
	MetricViews []string                `json:"metric_views"`
	Model       *modelRef               `json:"model,omitempty"`
	Counts      dashboardManifestCounts `json:"counts"`
	Pages       []dashboardManifestPage `json:"pages"`
	DetailTools map[string]string       `json:"detail_tools"`
}

type dashboardManifestCounts struct {
	Pages   int `json:"pages"`
	Visuals int `json:"visuals"`
	Tables  int `json:"tables"`
	Filters int `json:"filters"`
}

type dashboardManifestPage struct {
	ID          string                       `json:"id"`
	Title       string                       `json:"title"`
	Description string                       `json:"description,omitempty"`
	Components  []dashboardManifestComponent `json:"components"`
}

type dashboardManifestComponent struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Title string `json:"title,omitempty"`
}

func dashboardManifest(report semantic.Dashboard, model *semantic.Model, pages []dashboard.Page) dashboardManifestSummary {
	if pages == nil {
		pages = report.Pages
	}
	out := dashboardManifestSummary{
		ID:          report.ID,
		Title:       report.Title,
		Description: report.Description,
		MetricViews: report.MetricViews,
		Model:       modelSummary(model),
		Counts: dashboardManifestCounts{
			Pages:   len(pages),
			Visuals: len(report.Visuals),
			Tables:  len(report.Tables),
			Filters: len(report.Filters),
		},
		Pages: make([]dashboardManifestPage, 0, len(pages)),
		DetailTools: map[string]string{
			"metric_view": "describe_metric_view",
			"page_data":   "query_dashboard_page",
			"table_data":  "query_table",
		},
	}
	for _, page := range pages {
		pageSummary := dashboardManifestPage{
			ID:          page.ID,
			Title:       page.Title,
			Description: page.Description,
			Components:  make([]dashboardManifestComponent, 0, len(page.Visuals)),
		}
		for _, component := range page.Visuals {
			pageSummary.Components = append(pageSummary.Components, dashboardComponentSummary(component, report))
		}
		out.Pages = append(out.Pages, pageSummary)
	}
	return out
}

func dashboardComponentSummary(component dashboard.PageVisual, report semantic.Dashboard) dashboardManifestComponent {
	switch {
	case component.Visual != "":
		title := component.Title
		if title == "" {
			title = report.Visuals[component.Visual].Title
		}
		return dashboardManifestComponent{ID: component.ID, Kind: "visual", Ref: component.Visual, Title: title}
	case component.Table != "":
		title := component.Title
		if title == "" {
			title = report.Tables[component.Table].Title
		}
		return dashboardManifestComponent{ID: component.ID, Kind: "table", Ref: component.Table, Title: title}
	case component.Filter != "":
		title := component.Title
		if title == "" {
			title = report.Filters[component.Filter].Label
		}
		return dashboardManifestComponent{ID: component.ID, Kind: "filter", Ref: component.Filter, Title: title}
	default:
		kind := component.Kind
		if kind == "" {
			kind = "component"
		}
		return dashboardManifestComponent{ID: component.ID, Kind: kind, Title: component.Title}
	}
}
