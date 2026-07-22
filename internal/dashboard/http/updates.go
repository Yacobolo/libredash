package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	nethttp "net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	lddatastar "github.com/Yacobolo/leapview/internal/dashboard/datastar"
	dashboardstream "github.com/Yacobolo/leapview/internal/dashboard/stream"
	reportui "github.com/Yacobolo/leapview/internal/dashboard/ui"
	"github.com/Yacobolo/leapview/pkg/pagestream"
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
	clientID := pagestream.ClientIDFromRequest(r, strings.TrimSpace(r.URL.Query().Get("clientId")))
	streamInstanceID := strings.TrimSpace(r.URL.Query().Get("streamInstance"))
	if streamInstanceID == "" {
		streamInstanceID = fallbackStreamInstanceID()
	}
	streamID := lddatastar.StreamID(clientID, dashboardID, activePage.ID, streamInstanceID)
	request := command.Request{
		DashboardID: dashboardID,
		PageID:      activePage.ID,
		ModelID:     metrics.ModelIDForDashboard(dashboardID),
	}

	broker := h.Broker
	if broker == nil {
		broker = pagestream.NewBroker()
	}
	mailbox, unsubscribe := broker.Subscribe(streamID)
	defer unsubscribe()

	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(
		broker.TraceStore(), streamID, "dashboard.bootstrap",
	))
	bootstrap := reportui.BootstrapSignals(clientID, streamInstanceID, metrics.Catalog(), reportDefinition, model, pages, activePage, initialFilters)
	if presentation, ok := publicPresentationFromContext(r.Context()); ok {
		bootstrap = reportui.PublicBootstrapSignals(clientID, streamInstanceID, presentation.PublicID, presentation.Presentation, metrics.Catalog(), reportDefinition, model, pages, activePage, initialFilters)
	} else if hasClientAgentState(r) {
		delete(bootstrap, "agent")
		delete(bootstrap, "agentVisuals")
	} else if h.AgentBootstrap != nil {
		agentState := h.AgentBootstrap(r, metrics.Catalog().Workspace.ID)
		bootstrap["agent"] = agentState.Agent
		bootstrap["agentVisuals"] = agentState.Visuals
	}
	status := lddatastar.LoadingPatch()["status"].(map[string]any)
	environment := ""
	if h.Environment != nil {
		environment = h.Environment(r)
	}
	if h.DataRefreshedAt != nil {
		status["lastUpdated"] = h.DataRefreshedAt(r.Context(), metrics.Catalog().Workspace.ID, environment, request.ModelID)
	}
	bootstrap["status"] = status
	if err := updates.Patch(bootstrap); err != nil {
		return
	}

	registry := h.Coordinators
	if registry == nil {
		registry = dashboardstream.NewRegistry()
	}
	coordinator, closeCoordinator := registry.Open(streamID, r.Context(), func(event dashboardstream.RefreshEvent) {
		broker.PublishEnvelope(streamID, lddatastar.RefreshEventEnvelope(event))
	})
	defer closeCoordinator()
	h.observeRefreshes(coordinator, dashboardID, activePage.ID)
	service := command.Service{Metrics: metrics}
	registry.Bind(streamID, metrics.Catalog().Workspace.ID, environment, request.ModelID, func() {
		_, _ = coordinator.BeginPrepared(func(current dashboard.Filters) (dashboardstream.RefreshPreparation, error) {
			prepared, err := service.PrepareInitial(request, current)
			return streamPreparation(prepared), err
		}, func(preparation dashboardstream.RefreshPreparation) dashboardstream.RefreshWork {
			plan, _ := preparation.Plan.(command.RefreshPlan)
			return dashboardstream.TargetWork(metrics, dashboardstream.WorkRequest{
				DashboardID: dashboardID, PageID: activePage.ID, ModelID: request.ModelID,
				Filters: preparation.Filters, Plan: plan, EventObserved: h.RefreshEventObserved, CacheObserved: h.CacheObserved,
			})
		})
	})
	_, err := coordinator.BeginPrepared(func(dashboard.Filters) (dashboardstream.RefreshPreparation, error) {
		prepared, err := service.PrepareInitial(request, initialFilters)
		return streamPreparation(prepared), err
	}, func(preparation dashboardstream.RefreshPreparation) dashboardstream.RefreshWork {
		plan, _ := preparation.Plan.(command.RefreshPlan)
		return dashboardstream.TargetWork(metrics, dashboardstream.WorkRequest{
			DashboardID:   dashboardID,
			PageID:        activePage.ID,
			ModelID:       request.ModelID,
			Filters:       preparation.Filters,
			Plan:          plan,
			EventObserved: h.RefreshEventObserved,
			CacheObserved: h.CacheObserved,
		})
	})
	if err != nil {
		return
	}
	_ = updates.ForwardUpdates(r.Context(), mailbox)
}

func hasClientAgentState(r *nethttp.Request) bool {
	var signals struct {
		Agent *json.RawMessage `json:"agent"`
	}
	return pagestream.ReadSignals(r, &signals) == nil && signals.Agent != nil
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

func fallbackStreamInstanceID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return "server-stream"
}

func (h Handler) refreshObserver(dashboardID, pageID string) dashboardstream.SummaryObserver {
	logger := h.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(summary dashboardstream.RefreshSummary) {
		logger.Info("dashboard refresh",
			"event", "dashboard_refresh",
			"refreshId", summary.RefreshID,
			"generation", summary.Generation,
			"dashboard", dashboardID,
			"page", pageID,
			"command", summary.Command,
			"plannedTargets", summary.PlannedTargets,
			"targetSuccesses", summary.TargetSuccesses,
			"targetErrors", summary.TargetErrors,
			"queryCount", summary.QueryCount,
			"cancellationCount", summary.CancellationCount,
			"cancellationReason", summary.CancellationReason,
			"cacheOutcomes", summary.CacheOutcomes,
			"stageTimingsMs", summary.StageTimingsMs,
			"outcome", summary.Outcome,
		)
	}
}

func (h Handler) observeRefreshes(coordinator *dashboardstream.Coordinator, dashboardID, pageID string) {
	coordinator.SetStartObserver(h.RefreshStarted)
	logFinished := h.refreshObserver(dashboardID, pageID)
	coordinator.SetObserver(func(summary dashboardstream.RefreshSummary) {
		logFinished(summary)
		if h.RefreshFinished != nil {
			h.RefreshFinished(summary)
		}
	})
}
