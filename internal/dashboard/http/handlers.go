package http

import (
	"context"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	reportui "github.com/Yacobolo/libredash/internal/dashboard/ui"
	"github.com/Yacobolo/libredash/pkg/pagestream"
	"github.com/go-chi/chi/v5"
)

type Metrics interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	DefaultFilters(dashboardID string) dashboard.Filters
	ModelIDForDashboard(dashboardID string) string
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	Pages(dashboardID string) []dashboard.Page
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	RefreshMaterializations(ctx context.Context, modelID string) error
}

type Handler struct {
	Metrics             Metrics
	MetricsForWorkspace func(workspaceID string) (Metrics, bool)
	Broker              *pagestream.Broker
	CurrentPrincipalID  func(r *nethttp.Request) string
	CSRFToken           func(r *nethttp.Request) string
	ChromeDecorators    func(r *nethttp.Request) []reportui.ChromeDecorator
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
