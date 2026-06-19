package app

import (
	"context"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/starfederation/datastar-go/datastar"
)

func (s *Server) tableWindow(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	pageID := pageIDFromRequest(r, signals)
	filters := s.normalizeFilters(dashboardID, pageID, signals.Filters)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	clientID := clientStreamID(r, signals, dashboardID, pageID)

	table := s.queryTable(r.Context(), dashboardID, pageID, filters, request)
	if !isCanceledTable(table) {
		s.broker.publish(clientID, tablePatch(request.Table, table))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) chartSelect(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	pageID := pageIDFromRequest(r, signals)
	filters := s.normalizeFilters(dashboardID, pageID, signals.Filters).ToggleSelection(signals.VisualCommand)
	filters = s.normalizeFilters(dashboardID, pageID, filters)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageID)

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, pageID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearSelection(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	pageID := pageIDFromRequest(r, signals)
	filters := s.normalizeFilters(dashboardID, pageID, signals.Filters)
	filters.VisualSelections = nil
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageID)

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, pageID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetFilters(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	pageID := pageIDFromRequest(r, signals)
	filters := s.defaultFilters(dashboardID, pageID)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageID)

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, pageID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshMaterializations(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	pageID := pageIDFromRequest(r, signals)
	modelID := s.modelID(r, signals, dashboardID)
	filters := s.normalizeFilters(dashboardID, pageID, signals.Filters)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageID)

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	if err := s.metrics.RefreshMaterializations(r.Context(), modelID); err != nil {
		s.broker.publish(clientID, dashboardPatch(dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, pageID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) queryTable(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) dashboard.Table {
	table, err := s.metrics.QueryTablePage(ctx, dashboardID, pageID, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err)
	}
	return table
}

func isCanceledTable(table dashboard.Table) bool {
	message := strings.ToLower(table.Error)
	return strings.Contains(message, "context canceled") ||
		strings.Contains(message, "context cancelled") ||
		strings.Contains(message, "interrupt")
}

func (s *Server) defaultFilters(dashboardID, pageID string) dashboard.Filters {
	report, _, ok := s.metrics.Report(dashboardID)
	if !ok {
		return s.metrics.DefaultFilters(dashboardID)
	}
	page, ok := report.PageOrDefault(pageID)
	if !ok {
		return dashboard.Filters{}.WithDefaults()
	}
	return report.DefaultFiltersForPage(page.ID)
}

func (s *Server) normalizeFilters(dashboardID, pageID string, filters dashboard.Filters) dashboard.Filters {
	report, _, ok := s.metrics.Report(dashboardID)
	if ok {
		page, ok := report.PageOrDefault(pageID)
		if !ok {
			return dashboard.Filters{}.WithDefaults()
		}
		return report.NormalizeFiltersForPage(page.ID, filters)
	}
	defaults := s.metrics.DefaultFilters(dashboardID)
	filters = filters.WithDefaults()
	for name, control := range filters.Controls {
		defaults.Controls[name] = control
	}
	defaults.VisualSelections = append([]dashboard.VisualSelection{}, filters.VisualSelections...)
	return defaults.WithDefaults()
}

func tablePatch(name string, table dashboard.Table) signalPatch {
	return signalPatch{
		"tables": map[string]dashboard.Table{
			name: table,
		},
	}
}

func (s *Server) tablesPatch(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, baseRequest dashboard.TableRequest) signalPatch {
	report, _, ok := s.metrics.Report(dashboardID)
	if !ok {
		if baseRequest.Table == "" {
			return signalPatch{"tables": map[string]dashboard.Table{}}
		}
		return tablePatch(baseRequest.Table, s.queryTable(ctx, dashboardID, pageID, filters, baseRequest))
	}
	tables := map[string]dashboard.Table{}
	for _, name := range pageTableNames(report.Pages, pageID) {
		table := report.Tables[name]
		request := baseRequest
		request.Table = name
		request.Block = "all"
		request.Start = 0
		request.Count = dashboard.TableChunkSize
		request.Sort = table.DefaultSort
		tables[name] = s.queryTable(ctx, dashboardID, pageID, filters, request)
	}
	return signalPatch{"tables": tables}
}

func pageTableNames(pages []dashboard.Page, pageID string) []string {
	page, ok := activePageOrDefault(pages, pageID)
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

func activePageOrDefault(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
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

func loadingPatch(dataDir string) signalPatch {
	return signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": dataDir,
		},
	}
}
