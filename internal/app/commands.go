package app

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
	"github.com/starfederation/datastar-go/datastar"
)

func (s *Server) tableWindow(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	filters := s.normalizeFilters(dashboardID, signals.Filters)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	table := s.queryTable(r.Context(), dashboardID, filters, request)
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
	filters := s.normalizeFilters(dashboardID, signals.Filters).ToggleSelection(signals.ChartCommand)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearSelection(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	filters := s.normalizeFilters(dashboardID, signals.Filters)
	filters.VisualSelections = nil
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetFilters(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	filters := s.metrics.DefaultFilters(dashboardID)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshCache(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	modelID := s.modelID(r, signals, dashboardID)
	filters := s.normalizeFilters(dashboardID, signals.Filters)
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand).Reset()
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, loadingPatch(s.metrics.DataDir()))

	if err := s.metrics.RefreshCache(r.Context(), modelID); err != nil {
		s.broker.publish(clientID, dashboardPatch(dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, s.tablesPatch(r.Context(), dashboardID, filters, request))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) queryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) dashboard.Table {
	table, err := s.metrics.QueryTable(ctx, dashboardID, filters, request)
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

func (s *Server) normalizeFilters(dashboardID string, filters dashboard.Filters) dashboard.Filters {
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

func (s *Server) tablesPatch(ctx context.Context, dashboardID string, filters dashboard.Filters, baseRequest dashboard.TableRequest) signalPatch {
	report, _, ok := s.metrics.Report(dashboardID)
	if !ok {
		return tablePatch(baseRequest.Table, s.queryTable(ctx, dashboardID, filters, baseRequest))
	}
	tables := map[string]dashboard.Table{}
	for _, name := range sortedTableNames(report.Tables) {
		table := report.Tables[name]
		request := baseRequest
		request.Table = name
		request.Block = "all"
		request.Start = 0
		request.Count = dashboard.TableChunkSize
		request.Sort = table.DefaultSort
		tables[name] = s.queryTable(ctx, dashboardID, filters, request)
	}
	return signalPatch{"tables": tables}
}

func sortedTableNames(tables map[string]semantic.TableVisual) []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
