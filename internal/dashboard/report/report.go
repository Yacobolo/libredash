package report

import (
	"context"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
)

type Metrics interface {
	DefaultFilters(dashboardID string) dashboard.Filters
	Report(dashboardID string) (Dashboard, *semanticmodel.Model, bool)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
}

func ActivePage(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
	if len(pages) == 0 {
		return DefaultPage(), true
	}
	if pageID != "" {
		for _, page := range pages {
			if page.ID == pageID {
				return page.WithDefaults(), true
			}
		}
		return dashboard.Page{}, false
	}
	return pages[0].WithDefaults(), true
}

func ActivePageOrDefault(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
	if len(pages) == 0 {
		return dashboard.Page{}, false
	}
	if pageID != "" {
		for _, page := range pages {
			if page.ID == pageID {
				return page.WithDefaults(), true
			}
		}
	}
	return pages[0].WithDefaults(), true
}

func DefaultPage() dashboard.Page {
	return dashboard.Page{
		ID:     "overview",
		Title:  "Overview",
		Canvas: dashboard.PageCanvas{Width: 1366, Height: 940},
		Grid:   dashboard.PageGrid{Columns: 12, RowHeight: 48, Gap: 16, Padding: 16},
	}
}

func DefaultFilters(metrics Metrics, dashboardID, pageID string) dashboard.Filters {
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		return metrics.DefaultFilters(dashboardID)
	}
	page, ok := report.PageOrDefault(pageID)
	if !ok {
		return dashboard.Filters{}.WithDefaults()
	}
	return report.DefaultFiltersForPage(page.ID)
}

func NormalizeFilters(metrics Metrics, dashboardID, pageID string, filters dashboard.Filters) dashboard.Filters {
	report, _, ok := metrics.Report(dashboardID)
	if ok {
		page, ok := report.PageOrDefault(pageID)
		if !ok {
			return dashboard.Filters{}.WithDefaults()
		}
		return report.NormalizeFiltersForPage(page.ID, filters)
	}
	defaults := metrics.DefaultFilters(dashboardID)
	filters = filters.WithDefaults()
	for name, control := range filters.Controls {
		defaults.Controls[name] = control
	}
	defaults.Selections = append([]dashboard.InteractionSelection{}, filters.Selections...)
	return defaults.WithDefaults()
}

func QueryTable(ctx context.Context, metrics Metrics, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) dashboard.Table {
	table, err := metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err)
	}
	return table
}

func IsCanceledTable(table dashboard.Table) bool {
	message := strings.ToLower(table.Error)
	return strings.Contains(message, "context canceled") ||
		strings.Contains(message, "context cancelled") ||
		strings.Contains(message, "interrupt")
}

func Tables(ctx context.Context, metrics Metrics, dashboardID, pageID string, filters dashboard.Filters, baseRequest dashboard.TableRequest) map[string]dashboard.Table {
	report, _, ok := metrics.Report(dashboardID)
	if !ok {
		if baseRequest.Table == "" {
			return map[string]dashboard.Table{}
		}
		return map[string]dashboard.Table{
			baseRequest.Table: QueryTable(ctx, metrics, dashboardID, pageID, filters, baseRequest),
		}
	}
	tables := map[string]dashboard.Table{}
	for _, name := range PageTableNames(report.Pages, pageID) {
		table := report.Tables[name]
		request := baseRequest
		request.Table = name
		request.Block = "all"
		request.Start = 0
		request.Count = dashboard.TableChunkSize
		request.Sort = table.DefaultSort
		tables[name] = QueryTable(ctx, metrics, dashboardID, pageID, filters, request)
	}
	return tables
}

func PageTableNames(pages []dashboard.Page, pageID string) []string {
	page, ok := ActivePageOrDefault(pages, pageID)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	names := []string{}
	for _, visual := range page.Visuals {
		if visual.Table == "" {
			continue
		}
		if _, ok := seen[visual.Table]; ok {
			continue
		}
		seen[visual.Table] = struct{}{}
		names = append(names, visual.Table)
	}
	return names
}
