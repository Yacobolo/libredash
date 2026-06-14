package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/starfederation/datastar-go/datastar"
)

type queryMetrics interface {
	QueryDashboard(ctx context.Context, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	RefreshCache(ctx context.Context) error
	DataDir() string
	Pages() []dashboard.Page
	ModelGraph() dashboard.ModelGraph
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
	mux.HandleFunc("GET /pages/{page}", s.page)
	mux.HandleFunc("GET /model", s.model)
	mux.HandleFunc("GET /updates", s.updates)
	mux.HandleFunc("POST /commands/table-window", s.tableWindow)
	mux.HandleFunc("POST /commands/chart-select", s.chartSelect)
	mux.HandleFunc("POST /commands/clear-selection", s.clearSelection)
	mux.HandleFunc("POST /commands/refresh-cache", s.refreshCache)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	return mux
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, "")
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, r.PathValue("page"))
}

func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.ModelPage(s.metrics.ModelGraph()).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, pageID string) {
	clientID := ensureClientID(w, r)
	pages := s.metrics.Pages()
	activePage, ok := activePage(pages, pageID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.Page(s.metrics.DataDir(), clientID, pages, activePage).Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func activePage(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
	if len(pages) == 0 {
		return dashboard.Page{
			ID:     "overview",
			Title:  "Overview",
			Width:  1366,
			Height: 940,
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
	filters := signals.Filters.WithDefaults()
	clientID := clientIDFromRequest(r, signals)
	tableRequest := dashboard.DefaultTableRequest()

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

	patch, err := s.metrics.QueryDashboard(r.Context(), filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}

	if err := sse.MarshalAndPatchSignals(patch); err != nil {
		return
	}
	if err := sse.MarshalAndPatchSignals(tablePatch(tableRequest.Table, s.queryTable(r.Context(), filters, tableRequest))); err != nil {
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
			patch, err := s.metrics.QueryDashboard(r.Context(), filters)
			if err != nil {
				patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
			if err := sse.MarshalAndPatchSignals(tablePatch(tableRequest.Table, s.queryTable(r.Context(), filters, tableRequest))); err != nil {
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
	filters := signals.Filters.WithDefaults()
	request := signals.TableCommand.WithDefaults()
	clientID := clientIDFromRequest(r, signals)

	s.broker.publish(clientID, tableLoadingPatch(request))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), filters, request)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) chartSelect(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters := signals.Filters.ToggleSelection(signals.ChartCommand)
	request := signals.TableCommand.WithDefaults()
	clientID := clientIDFromRequest(r, signals)

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), filters, request)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearSelection(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters := signals.Filters.WithDefaults()
	filters.VisualSelections = nil
	request := signals.TableCommand.WithDefaults()
	clientID := clientIDFromRequest(r, signals)

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	patch, err := s.metrics.QueryDashboard(r.Context(), filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), filters, request)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) refreshCache(w http.ResponseWriter, r *http.Request) {
	signals := dashboard.Signals{}
	if err := datastar.ReadSignals(r, &signals); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters := signals.Filters.WithDefaults()
	request := signals.TableCommand.WithDefaults()
	clientID := clientIDFromRequest(r, signals)

	s.broker.publish(clientID, signalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"dataDirectory": s.metrics.DataDir(),
		},
	})

	if err := s.metrics.RefreshCache(r.Context()); err != nil {
		s.broker.publish(clientID, dashboardPatch(dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	patch, err := s.metrics.QueryDashboard(r.Context(), filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}
	s.broker.publish(clientID, dashboardPatch(patch))
	s.broker.publish(clientID, tablePatch(request.Table, s.queryTable(r.Context(), filters, request)))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) queryTable(ctx context.Context, filters dashboard.Filters, request dashboard.TableRequest) dashboard.Table {
	table, err := s.metrics.QueryTable(ctx, filters, request)
	if err != nil {
		return dashboard.EmptyTable(request, err)
	}
	return table
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
		"filters": patch.Filters,
		"status":  patch.Status,
		"kpis":    patch.KPIs,
		"charts":  patch.Charts,
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
