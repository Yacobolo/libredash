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
	MetricViews() []dashboard.MetricViewSummary
	MetricView(id string) (dashboard.MetricViewDetail, bool)
	ModelGraph(modelID string) (dashboard.ModelGraph, bool)
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
				return map[string]any{"dashboards": s.metrics.Catalog().Dashboards}, nil
			}),
		},
		{
			Name:        "describe_dashboard",
			Description: "Describe a dashboard, its pages, metric views, visuals, and tables.",
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
				return map[string]any{
					"id":           report.ID,
					"title":        report.Title,
					"description":  report.Description,
					"metric_views": report.MetricViews,
					"model":        modelSummary(model),
					"pages":        s.metrics.Pages(input.DashboardID),
					"visuals":      report.Visuals,
					"tables":       report.Tables,
				}, nil
			}),
		},
		{
			Name:        "list_metric_views",
			Description: "List metric views available in the current workspace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, _ json.RawMessage) (any, error) {
				return map[string]any{"metric_views": s.metrics.MetricViews()}, nil
			}),
		},
		{
			Name:        "describe_metric_view",
			Description: "Describe dimensions, measures, and dashboard usage for a metric view.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"metric_view_id":{"type":"string"}},"required":["metric_view_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					MetricViewID string `json:"metric_view_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				view, ok := s.metrics.MetricView(input.MetricViewID)
				if !ok {
					return nil, fmt.Errorf("metric view %q not found", input.MetricViewID)
				}
				return view, nil
			}),
		},
		{
			Name:        "describe_model",
			Description: "Describe a semantic model graph by model ID.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"model_id":{"type":"string"}},"required":["model_id"],"additionalProperties":false}`),
			Handler: s.tool(func(ctx context.Context, raw json.RawMessage) (any, error) {
				var input struct {
					ModelID string `json:"model_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil {
					return nil, err
				}
				graph, ok := s.metrics.ModelGraph(input.ModelID)
				if !ok {
					return nil, fmt.Errorf("model %q not found", input.ModelID)
				}
				return graph, nil
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

func modelSummary(model *semantic.Model) map[string]any {
	if model == nil {
		return nil
	}
	return map[string]any{
		"id":    model.Name,
		"title": model.Title,
	}
}
