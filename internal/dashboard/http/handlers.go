package http

import (
	"context"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	"github.com/Yacobolo/leapview/internal/dashboard/consumer"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	"github.com/Yacobolo/leapview/internal/dashboard/report"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	reportui "github.com/Yacobolo/leapview/internal/dashboard/ui"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/Yacobolo/leapview/internal/ui"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
	"github.com/Yacobolo/leapview/pkg/pagestream"
	"github.com/go-chi/chi/v5"
)

type publicPresentationContextKey struct{}

type PublicPresentation struct {
	PublicID     string
	Presentation string
}

func WithPublicPresentation(ctx context.Context, value PublicPresentation) context.Context {
	return context.WithValue(ctx, publicPresentationContextKey{}, value)
}

func publicPresentationFromContext(ctx context.Context) (PublicPresentation, bool) {
	value, ok := ctx.Value(publicPresentationContextKey{}).(PublicPresentation)
	return value, ok
}

type Metrics interface {
	consumer.Executor
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	DefaultFilters(dashboardID string) dashboard.Filters
	ModelIDForDashboard(dashboardID string) string
	NormalizeVisualizationWindow(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	Pages(dashboardID string) []dashboard.Page
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryVisualizationWindow(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request visualizationir.VisualizationWindowRequest) (visualizationir.VisualizationEnvelope, error)
	Report(dashboardID string) (dashboarddefinition.Definition, *semanticmodel.Model, bool)
	VisualizationDefinition(dashboardID, visualID string) (visualizationdefinition.Definition, bool)
}

type SignalBroker interface {
	Subscribe(streamID string) (<-chan pagestream.SignalPatch, func())
	PublishEnvelope(streamID string, envelope pagestream.Envelope)
	TraceStore() *pagestream.TraceStore
}

type SharedCommandPrepare func(
	r *nethttp.Request,
	request command.Request,
	signals dashboard.Signals,
	prepare func(dashboard.Filters) (command.PreparedRefresh, error),
) (command.PreparedRefresh, uint64, error)

type Handler struct {
	Metrics              Metrics
	MetricsForWorkspace  func(workspaceID string) (Metrics, bool)
	AnalyticalContext    func(context.Context) context.Context
	Broker               SignalBroker
	Coordinators         *dashboardstream.Registry
	Logger               *slog.Logger
	RefreshStarted       dashboardstream.StartObserver
	RefreshFinished      dashboardstream.SummaryObserver
	RefreshEventObserved dashboardstream.EventPublisher
	CacheObserved        dataquery.CacheOutcomeObserver
	CurrentPrincipalID   func(r *nethttp.Request) string
	AuthorizeListObject  func(ctx context.Context, principalID string, object access.ObjectRef) (bool, error)
	CSRFToken            func(r *nethttp.Request) string
	ChromeDecorators     func(r *nethttp.Request) []reportui.ChromeDecorator
	Environment          func(*nethttp.Request) string
	DataRefreshedAt      func(context.Context, string, string, string) string
	CommandGuard         func(*nethttp.Request, Metrics, command.Request, dashboard.Signals) error
	SharedCommandPrepare SharedCommandPrepare
	AgentBootstrap       func(*nethttp.Request, string) ui.ChatViewState
}

func (h Handler) analyticalContext(ctx context.Context) context.Context {
	if h.AnalyticalContext == nil {
		return ctx
	}
	return h.AnalyticalContext(ctx)
}

func (h Handler) filterAuthorizedDashboards(ctx context.Context, principalID, workspaceID string, rows []api.DashboardSummary) ([]api.DashboardSummary, error) {
	if h.AuthorizeListObject == nil {
		return rows, nil
	}
	out := make([]api.DashboardSummary, 0, len(rows))
	for _, row := range rows {
		object := access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, row.ID, access.WorkspaceObject(workspaceID))
		allowed, err := h.AuthorizeListObject(ctx, principalID, object)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, row)
		}
	}
	return out, nil
}

func DashboardObjectRefs(r *nethttp.Request, workspaceID string) []access.ObjectRef {
	objects := []access.ObjectRef{}
	if dashboardID := strings.TrimSpace(chi.URLParam(r, "dashboard")); dashboardID != "" {
		objects = append(objects, access.ItemObjectWithParent(access.SecurableDashboard, workspaceID, dashboardID, access.WorkspaceObject(workspaceID)))
	}
	if strings.TrimSpace(workspaceID) != "" {
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	return objects
}

func (h Handler) Dashboard(w nethttp.ResponseWriter, r *nethttp.Request) {
	workspaceID := chi.URLParam(r, "workspace")
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	dashboardID := chi.URLParam(r, "dashboard")
	if workspaceID == "" {
		workspaceID = metrics.Catalog().Workspace.ID
	}
	pages := metrics.Pages(dashboardID)
	if len(pages) == 0 {
		nethttp.NotFound(w, r)
		return
	}
	nethttp.Redirect(w, r, "/workspaces/"+workspaceID+"/dashboards/"+dashboardID+"/pages/"+pages[0].ID, nethttp.StatusFound)
}

func (h Handler) Page(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.RenderPage(w, r, chi.URLParam(r, "dashboard"), chi.URLParam(r, "page"))
}

func (h Handler) RenderPage(w nethttp.ResponseWriter, r *nethttp.Request, dashboardID, pageID string) {
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	clientID := pagestream.EnsureClientID(w, r)
	reportDefinition, model, ok := metrics.Report(dashboardID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	pages := metrics.Pages(dashboardID)
	activePage, ok := report.ActivePage(pages, pageID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(nethttp.StatusOK)
	initialFilters := reportDefinition.FiltersFromURLForPage(activePage.ID, r.URL.Query())
	csrfToken := ""
	if h.CSRFToken != nil {
		csrfToken = h.CSRFToken(r)
	}
	var chromeDecorators []reportui.ChromeDecorator
	if h.ChromeDecorators != nil {
		chromeDecorators = h.ChromeDecorators(r)
	}
	if err := reportui.Page(clientID, csrfToken, metrics.Catalog(), reportDefinition, model, pages, activePage, initialFilters, chromeDecorators...).Render(w); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
	}
}

func (h Handler) metricsForRequest(r *nethttp.Request) (Metrics, bool) {
	workspaceID := chi.URLParam(r, "workspace")
	if workspaceID == "" {
		workspaceID = r.URL.Query().Get("workspace")
	}
	if workspaceID != "" && h.MetricsForWorkspace != nil {
		return h.MetricsForWorkspace(workspaceID)
	}
	if h.Metrics == nil {
		return nil, false
	}
	return h.Metrics, true
}
