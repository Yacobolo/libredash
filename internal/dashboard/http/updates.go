package http

import (
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	"github.com/Yacobolo/libredash/internal/dashboard/stream"
	reportui "github.com/Yacobolo/libredash/internal/dashboard/ui"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

func (h Handler) Updates(w nethttp.ResponseWriter, r *nethttp.Request) {
	metrics, ok := h.metricsForRequest(r)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	dashboardID := strings.TrimSpace(r.URL.Query().Get("dashboard"))
	if dashboardID == "" {
		dashboardID = metrics.DefaultDashboardID()
	}
	pageID := strings.TrimSpace(r.URL.Query().Get("page"))
	reportDefinition, model, ok := metrics.Report(dashboardID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	pages := metrics.Pages(dashboardID)
	activePage, ok := streamActivePage(pages, pageID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	initialFilters := reportDefinition.FiltersFromURLForPage(activePage.ID, r.URL.Query())
	clientID := pagestream.ClientIDFromRequest(r, "")
	request := stream.SnapshotRequest{
		DashboardID: dashboardID,
		PageID:      activePage.ID,
		Filters:     initialFilters,
	}

	updates := pagestream.NewSignalStream(w, r)
	bootstrap := reportui.BootstrapSignals(clientID, metrics.Catalog(), reportDefinition, model, pages, activePage, initialFilters)
	bootstrap["status"] = lddatastar.LoadingPatch()["status"]
	if err := updates.Patch(bootstrap); err != nil {
		return
	}
	snapshot := stream.Service{Metrics: metrics}.Snapshot(r.Context(), request)
	for _, patch := range lddatastar.SnapshotPatches(snapshot) {
		if err := updates.Patch(patch); err != nil {
			return
		}
	}
	_ = updates.Forward(r.Context(), h.Broker, lddatastar.StreamID(clientID, dashboardID, activePage.ID))
}

func streamActivePage(pages []dashboard.Page, pageID string) (dashboard.Page, bool) {
	if pageID == "" && len(pages) > 0 {
		return pages[0], true
	}
	for _, page := range pages {
		if page.ID == pageID {
			return page, true
		}
	}
	return dashboard.Page{}, false
}
