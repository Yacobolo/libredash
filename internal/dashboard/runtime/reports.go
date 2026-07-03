package runtime

import (
	"context"
	"fmt"
	"sort"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type ReportService struct {
	workspace *workspace.Definition
	defaultID string
}

func (m *Service) DefaultDashboardID() string {
	return m.reports.DefaultDashboardID()
}

func (m *Service) ModelIDForDashboard(dashboardID string) string {
	return m.reports.ModelIDForDashboard(dashboardID)
}

func (m *Service) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	return m.reports.Report(dashboardID)
}

func (m *Service) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	return m.reports.SemanticModel(modelID)
}

func (m *Service) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	result, err := m.ExecuteDataQuery(ctx, reportAggregateDataQuery(modelID, request))
	return reportRowsFromDataQuery(result.Rows), err
}

func (m *Service) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	result, err := m.ExecuteDataQuery(ctx, reportRowDataQuery(modelID, request, false))
	return reportRowsFromDataQuery(result.Rows), err
}

func (m *Service) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if request.WorkspaceID == "" && m.reports != nil && m.reports.workspace != nil {
		request.WorkspaceID = m.reports.workspace.Catalog.Workspace.ID
	}
	return dataquery.ExecuteAudited(ctx, request, func(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
		runtime, err := m.semanticRuntime(request.ModelID)
		if err != nil {
			return dataquery.Result{}, err
		}
		m.mu.RLock()
		defer m.mu.RUnlock()
		return runtime.data.ExecuteDataQuery(ctx, request)
	})
}

func (m *Service) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	return m.reports.NormalizeTableRequest(dashboardID, request)
}

func (m *Service) DefaultFilters(dashboardID string) dashboard.Filters {
	return m.reports.DefaultFilters(dashboardID)
}

func (m *Service) Pages(dashboardID string) []dashboard.Page {
	return m.reports.Pages(dashboardID)
}

func (s *ReportService) DefaultDashboardID() string {
	return s.defaultID
}

func (s *ReportService) ModelIDForDashboard(dashboardID string) string {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return ""
	}
	if report.SemanticModel != "" {
		return report.SemanticModel
	}
	return ""
}

func (s *ReportService) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return reportdef.Dashboard{}, nil, false
	}
	if report.SemanticModel != "" {
		model, ok := s.workspace.Models[report.SemanticModel]
		if !ok {
			return reportdef.Dashboard{}, nil, false
		}
		return *report, model, true
	}
	return reportdef.Dashboard{}, nil, false
}

func (s *ReportService) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	model, ok := s.workspace.Models[modelID]
	return model, ok
}

func (s *ReportService) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return request.WithDefaults()
	}
	defaults := dashboard.TableRequest{Block: "all", Start: 0, Count: dashboard.TableChunkSize}
	if table, ok := report.Tables["orders"]; ok && table.KindOrDefault() == "data_table" {
		defaults.Table = "orders"
		defaults.Sort = table.DefaultSort
	} else {
		for _, name := range sortedKeys(report.Tables) {
			table := report.Tables[name]
			if table.KindOrDefault() != "data_table" {
				continue
			}
			defaults.Table = name
			defaults.Sort = table.DefaultSort
			break
		}
	}
	if defaults.Table == "" {
		defaults = dashboard.DefaultTableRequest()
	}
	if request.Table == "" {
		request.Table = defaults.Table
	}
	if request.Block == "" {
		request.Block = defaults.Block
	}
	if request.Block != "all" && request.Block != "a" && request.Block != "b" && request.Block != "c" {
		request.Block = defaults.Block
	}
	if request.Count <= 0 {
		request.Count = defaults.Count
	}
	if request.Count > dashboard.TableMaxRequestCount {
		request.Count = dashboard.TableMaxRequestCount
	}
	if request.Start < 0 {
		request.Start = 0
	}
	if request.Sort.Key == "" {
		request.Sort = defaults.Sort
	}
	if request.Sort.Direction != "asc" && request.Sort.Direction != "desc" {
		if defaults.Sort.Direction != "" {
			request.Sort.Direction = defaults.Sort.Direction
		} else {
			request.Sort.Direction = "desc"
		}
	}
	return request
}

func (s *ReportService) DefaultFilters(dashboardID string) dashboard.Filters {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return dashboard.Filters{}.WithDefaults()
	}
	return report.DefaultFilters()
}

func (s *ReportService) Pages(dashboardID string) []dashboard.Page {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return nil
	}
	pages := make([]dashboard.Page, len(report.Pages))
	for i, page := range report.Pages {
		pages[i] = page.WithDefaults()
	}
	return pages
}

func (s *ReportService) reportRuntime(dashboardID string, runtimes map[string]*modelRuntime) (*reportdef.Dashboard, *modelRuntime, error) {
	report, ok := s.workspace.Dashboards[dashboardID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown dashboard %q", dashboardID)
	}
	if report.SemanticModel == "" {
		return nil, nil, fmt.Errorf("dashboard %q has no semantic model", dashboardID)
	}
	runtime, ok := runtimes[report.SemanticModel]
	if !ok {
		return nil, nil, fmt.Errorf("unknown semantic model %q", report.SemanticModel)
	}
	return report, runtime, nil
}

func (m *Service) semanticRuntime(modelID string) (*modelRuntime, error) {
	runtime, ok := m.runtimes[modelID]
	if !ok {
		return nil, fmt.Errorf("unknown semantic model %q", modelID)
	}
	if !runtime.ready {
		return nil, runtime.missing
	}
	return runtime, nil
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
