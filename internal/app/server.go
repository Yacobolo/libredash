package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/starfederation/datastar-go/datastar"
)

type queryMetrics interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	ModelIDForDashboard(dashboardID string) string
	Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool)
	DefaultFilters(dashboardID string) dashboard.Filters
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	RefreshCache(ctx context.Context, modelID string) error
	DataDir() string
	Pages(dashboardID string) []dashboard.Page
	ModelGraph(modelID string) (dashboard.ModelGraph, bool)
}

type Server struct {
	metrics queryMetrics
	broker  *broker
}

func New(metrics queryMetrics) *Server {
	return &Server{metrics: metrics, broker: newBroker()}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /dashboards/{dashboard}", s.dashboard)
	mux.HandleFunc("GET /dashboards/{dashboard}/pages/{page}", s.page)
	mux.HandleFunc("GET /models/{model}", s.model)
	mux.HandleFunc("GET /updates", s.updates)
	mux.HandleFunc("POST /commands/table-window", s.tableWindow)
	mux.HandleFunc("POST /commands/chart-select", s.chartSelect)
	mux.HandleFunc("POST /commands/clear-selection", s.clearSelection)
	mux.HandleFunc("POST /commands/reset-filters", s.resetFilters)
	mux.HandleFunc("POST /commands/refresh-cache", s.refreshCache)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	return mux
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.CatalogPage(s.metrics.Catalog()).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	dashboardID := r.PathValue("dashboard")
	pages := s.metrics.Pages(dashboardID)
	if len(pages) == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboards/"+dashboardID+"/pages/"+pages[0].ID, http.StatusFound)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, r.PathValue("dashboard"), r.PathValue("page"))
}

func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	model, ok := s.metrics.ModelGraph(r.PathValue("model"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ModelPage(s.metrics.Catalog(), model).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, dashboardID, pageID string) {
	clientID := ensureClientID(w, r)
	report, model, ok := s.metrics.Report(dashboardID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	pages := s.metrics.Pages(dashboardID)
	activePage, ok := activePage(pages, pageID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	initialFilters := report.FiltersFromURL(r.URL.Query())
	if err := ui.Page(s.metrics.DataDir(), clientID, s.metrics.Catalog(), report, model, pages, activePage, initialFilters).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func activePage(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
	if len(pages) == 0 {
		return dashboard.Page{
			ID:     "overview",
			Title:  "Overview",
			Canvas: dashboard.PageCanvas{Width: 1366, Height: 940},
			Grid:   dashboard.PageGrid{Columns: 12, RowHeight: 48, Gap: 16, Padding: 16},
		}, true
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

func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dashboardID := s.dashboardID(r, signals)
	filters := s.normalizeFilters(dashboardID, signals.Filters)
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))
	tableRequest := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)

	sse := datastar.NewSSE(w, r)
	updates, unsubscribe := s.broker.subscribe(clientID)
	defer unsubscribe()

	_ = sse.MarshalAndPatchSignals(map[string]any{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}

	if err := sse.MarshalAndPatchSignals(patch); err != nil {
		return
	}
	if err := sse.MarshalAndPatchSignals(tablePatch(tableRequest.Table, s.queryTable(r.Context(), dashboardID, filters, tableRequest))); err != nil {
		return
	}

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case patch := <-updates:
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
		case <-ticker.C:
			patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
			if err != nil {
				patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
			if err := sse.MarshalAndPatchSignals(tablePatch(tableRequest.Table, s.queryTable(r.Context(), dashboardID, filters, tableRequest))); err != nil {
				return
			}
		}
	}
}

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

	s.broker.publish(clientID, tableLoadingPatch(request))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), dashboardID, filters, request)))
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
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), dashboardID, filters, request)))
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
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), dashboardID, filters, request)))
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
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	request.Offset = 0
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), dashboardID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), dashboardID, filters, request)))
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
	request := s.metrics.NormalizeTableRequest(dashboardID, signals.TableCommand)
	clientID := clientStreamID(r, signals, dashboardID, pageIDFromRequest(r, signals))

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

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
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), dashboardID, filters, request)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) queryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) dashboard.Table {
	table, err := s.metrics.QueryTable(ctx, dashboardID, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err)
	}
	return table
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

func dashboardPatch(patch dashboard.Patch) signalPatch {
	return signalPatch{
		"filters":       patch.Filters,
		"filterOptions": patch.FilterOptions,
		"status":        patch.Status,
		"kpis":          patch.KPIs,
		"charts":        patch.Charts,
	}
}

func tableLoadingPatch(request dashboard.TableRequest) signalPatch {
	request = request.WithDefaults()
	return signalPatch{
		"tables": map[string]any{
			request.Table: map[string]any{
				"loading": true,
				"error":   "",
				"window": dashboard.TableWindow{
					Offset: request.Offset,
					Limit:  request.Limit,
				},
				"sort": request.Sort,
			},
		},
	}
}

func ensureClientID(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie("ld_client_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	clientID := newClientID()
	http.SetCookie(w, &http.Cookie{
		Name:     "ld_client_id",
		Value:    clientID,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
	return clientID
}

func (s *Server) dashboardID(r *http.Request, signals dashboard.Signals) string {
	if id := r.URL.Query().Get("dashboard"); id != "" {
		return id
	}
	if signals.Runtime.DashboardID != "" {
		return signals.Runtime.DashboardID
	}
	return s.metrics.DefaultDashboardID()
}

func pageIDFromRequest(r *http.Request, signals dashboard.Signals) string {
	if id := r.URL.Query().Get("page"); id != "" {
		return id
	}
	if signals.Runtime.PageID != "" {
		return signals.Runtime.PageID
	}
	return ""
}

func (s *Server) modelID(r *http.Request, signals dashboard.Signals, dashboardID string) string {
	if id := r.URL.Query().Get("model"); id != "" {
		return id
	}
	if signals.Runtime.ModelID != "" {
		return signals.Runtime.ModelID
	}
	return s.metrics.ModelIDForDashboard(dashboardID)
}

func clientStreamID(r *http.Request, signals dashboard.Signals, dashboardID, pageID string) string {
	return clientIDFromRequest(r, signals) + ":" + dashboardID + ":" + pageID
}

func clientIDFromRequest(r *http.Request, signals dashboard.Signals) string {
	if signals.Runtime.ClientID != "" {
		return signals.Runtime.ClientID
	}
	cookie, err := r.Cookie("ld_client_id")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return "default"
}

func newClientID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(bytes[:])
}
