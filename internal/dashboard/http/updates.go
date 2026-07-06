package http

import (
	nethttp "net/http"

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
	signals, ok := h.readSignals(w, r)
	if !ok {
		return
	}
	dashboardID := lddatastar.DashboardID(r, signals, metrics.DefaultDashboardID())
	pageID := lddatastar.PageID(r, signals)
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
	if len(signals.Filters.Controls) > 0 || len(signals.Filters.Selections) > 0 {
		initialFilters = signals.Filters
	}
	clientID := lddatastar.ClientStreamID(r, signals, dashboardID, pageID)
	request := stream.SnapshotRequest{
		DashboardID:  dashboardID,
		PageID:       activePage.ID,
		Filters:      initialFilters,
		TableCommand: signals.TableCommand,
	}

	updates := pagestream.NewSignalStream(w, r)
	bootstrap := reportui.BootstrapSignals(metrics.DataDir(), pagestream.ClientIDFromRequest(r, signals.Runtime.ClientID), metrics.Catalog(), reportDefinition, model, pages, activePage, initialFilters)
	bootstrap["status"] = lddatastar.LoadingPatch(metrics.DataDir())["status"]
	if err := updates.Patch(bootstrap); err != nil {
		return
	}
	snapshot := stream.Service{Metrics: metrics}.Snapshot(r.Context(), request)
	for _, patch := range lddatastar.SnapshotPatches(snapshot) {
		if err := updates.Patch(patch); err != nil {
			return
		}
	}
	_ = updates.Forward(r.Context(), h.Broker, clientID)
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
