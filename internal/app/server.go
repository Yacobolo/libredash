package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/Yacobolo/libredash/internal/semantic"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
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
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	RefreshMaterializations(ctx context.Context, modelID string) error
	DataDir() string
	Pages(dashboardID string) []dashboard.Page
}

type Server struct {
	metrics            queryMetrics
	broker             *broker
	store              *platform.Store
	auth               *Auth
	reloader           runtimeReloader
	artifactDir        string
	defaultWorkspaceID string
	rateLimits         RateLimitConfig
	securityHeaders    SecurityHeadersConfig
	requestLogging     bool
	logger             *slog.Logger
}

func New(metrics queryMetrics) *Server {
	return &Server{metrics: metrics, broker: newBroker(), logger: slog.Default()}
}

type Options struct {
	Store              *platform.Store
	Auth               *Auth
	Reloader           runtimeReloader
	ArtifactDir        string
	DefaultWorkspaceID string
	RateLimits         RateLimitConfig
	SecurityHeaders    SecurityHeadersConfig
	RequestLogging     bool
	Logger             *slog.Logger
}

func NewWithOptions(metrics queryMetrics, options Options) *Server {
	server := New(metrics)
	server.store = options.Store
	server.auth = options.Auth
	server.reloader = options.Reloader
	server.artifactDir = options.ArtifactDir
	server.defaultWorkspaceID = options.DefaultWorkspaceID
	server.rateLimits = options.RateLimits
	server.securityHeaders = options.SecurityHeaders
	server.requestLogging = options.RequestLogging
	if options.Logger != nil {
		server.logger = options.Logger
	}
	return server
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

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := ui.LoginPage().Render(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	dashboardID := chi.URLParam(r, "dashboard")
	pages := s.metrics.Pages(dashboardID)
	if len(pages) == 0 {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboards/"+dashboardID+"/pages/"+pages[0].ID, http.StatusFound)
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, r, chi.URLParam(r, "dashboard"), chi.URLParam(r, "page"))
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
	initialFilters := report.FiltersFromURLForPage(activePage.ID, r.URL.Query())
	csrfToken := ""
	if s.auth != nil {
		csrfToken = csrf.Token(r)
	}
	if err := ui.Page(s.metrics.DataDir(), clientID, csrfToken, s.metrics.Catalog(), report, model, pages, activePage, initialFilters).Render(w); err != nil {
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
	pageID := pageIDFromRequest(r, signals)
	filters := s.normalizeFilters(dashboardID, pageID, signals.Filters)
	clientID := clientStreamID(r, signals, dashboardID, pageID)
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

	patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
	if err != nil {
		patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
	}

	if err := sse.MarshalAndPatchSignals(patch); err != nil {
		return
	}
	if err := sse.MarshalAndPatchSignals(s.tablesPatch(r.Context(), dashboardID, pageID, filters, tableRequest)); err != nil {
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
			patch, err := s.metrics.QueryDashboardPage(r.Context(), dashboardID, pageID, filters)
			if err != nil {
				patch = dashboard.EmptyPatch(filters, s.metrics.DataDir(), err)
			}
			if err := sse.MarshalAndPatchSignals(patch); err != nil {
				return
			}
			if err := sse.MarshalAndPatchSignals(s.tablesPatch(r.Context(), dashboardID, pageID, filters, tableRequest)); err != nil {
				return
			}
		}
	}
}

func dashboardPatch(patch dashboard.Patch) signalPatch {
	return signalPatch{
		"filters":       patch.Filters,
		"filterOptions": patch.FilterOptions,
		"status":        patch.Status,
		"visuals":       patch.Visuals,
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
