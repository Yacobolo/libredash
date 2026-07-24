package report

import (
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
)

type Metrics interface {
	DefaultFilters(dashboardID string) dashboard.Filters
	Report(dashboardID string) (dashboarddefinition.Definition, *semanticmodel.Model, bool)
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
	defaults.Selections = append([]dashboard.InteractionSelection{}, filters.Selections...)
	defaults.SpatialSelections = append([]dashboard.SpatialInteractionSelection{}, filters.SpatialSelections...)
	if filters.CompiledState != nil {
		state := dashboardfilter.CloneState(*filters.CompiledState)
		defaults.CompiledState = &state
	}
	defaults.ServingStateID = filters.ServingStateID
	return defaults.WithDefaults()
}
