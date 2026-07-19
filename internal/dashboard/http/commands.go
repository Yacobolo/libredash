package http

import (
	"context"
	nethttp "net/http"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

type commandPrepare func(command.Service, command.Request, dashboard.Filters) (command.PreparedRefresh, error)

func (h Handler) TableWindow(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(service command.Service, request command.Request, current dashboard.Filters) (command.PreparedRefresh, error) {
		return service.PrepareTableWindow(request, current)
	})
}

func (h Handler) Select(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(service command.Service, request command.Request, current dashboard.Filters) (command.PreparedRefresh, error) {
		return service.PrepareSelect(request, current)
	})
}

func (h Handler) ClearSelection(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(service command.Service, request command.Request, current dashboard.Filters) (command.PreparedRefresh, error) {
		return service.PrepareClearSelection(request, current)
	})
}

func (h Handler) Reload(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(service command.Service, request command.Request, current dashboard.Filters) (command.PreparedRefresh, error) {
		// Filter controls are authored by the client event, while selections stay
		// coordinator-owned so a stale signal post cannot resurrect or erase a
		// rapid interaction command.
		current.Controls = request.Filters.Controls
		return service.PrepareReload(request, current)
	})
}

func (h Handler) ResetFilters(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(service command.Service, request command.Request, _ dashboard.Filters) (command.PreparedRefresh, error) {
		return service.PrepareResetFilters(request)
	})
}

func (h Handler) handleCommand(w nethttp.ResponseWriter, r *nethttp.Request, prepare commandPrepare) {
	h.handleCommandWithBefore(w, r, prepare, nil)
}

func (h Handler) handleCommandWithBefore(w nethttp.ResponseWriter, r *nethttp.Request, prepare commandPrepare, before func(Metrics, command.Request) func(context.Context) error) {
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
	modelID := lddatastar.ModelID(r, signals, dashboardID, metrics.ModelIDForDashboard)
	streamID := lddatastar.ClientStreamID(r, signals, dashboardID, pageID)
	request := command.Request{
		DashboardID:        dashboardID,
		PageID:             pageID,
		ModelID:            modelID,
		Filters:            signals.Filters,
		TableCommand:       signals.TableCommand,
		InteractionCommand: signals.InteractionCommand,
	}

	registry := h.Coordinators
	if registry == nil {
		registry = dashboardstream.NewRegistry()
	}
	broker := h.Broker
	if broker == nil {
		broker = pagestream.NewBroker()
	}
	coordinator := registry.Ensure(streamID, context.WithoutCancel(r.Context()), func(event dashboardstream.RefreshEvent) {
		broker.PublishEnvelope(streamID, lddatastar.RefreshEventEnvelope(event))
	})
	h.observeRefreshes(coordinator, dashboardID, pageID)
	_, err := coordinator.BeginPrepared(func(current dashboard.Filters) (dashboardstream.RefreshPreparation, error) {
		prepared, err := prepare(command.Service{Metrics: metrics}, request, current)
		return streamPreparation(prepared), err
	}, func(preparation dashboardstream.RefreshPreparation) dashboardstream.RefreshWork {
		plan, _ := preparation.Plan.(command.RefreshPlan)
		workRequest := dashboardstream.WorkRequest{
			DashboardID:   dashboardID,
			PageID:        pageID,
			ModelID:       modelID,
			Filters:       preparation.Filters,
			Plan:          plan,
			EventObserved: h.RefreshEventObserved,
			CacheObserved: h.CacheObserved,
		}
		if before != nil {
			workRequest.Before = before(metrics, request)
		}
		return dashboardstream.TargetWork(metrics, workRequest)
	})
	if err != nil {
		// Invalid commands still form a generation so the canonical filters and
		// scoped failure are delivered through the page stream.
		_, _ = coordinator.BeginPrepared(func(current dashboard.Filters) (dashboardstream.RefreshPreparation, error) {
			return dashboardstream.RefreshPreparation{Filters: current, Command: "invalid_command"}, nil
		}, func(dashboardstream.RefreshPreparation) dashboardstream.RefreshWork {
			return func(ctx context.Context, publish dashboardstream.RefreshPublisher) {
				if ctx.Err() == nil {
					publish(dashboardstream.RefreshEvent{Type: dashboardstream.RefreshEventTargetError, Target: "refresh", Err: err})
				}
			}
		})
	}
	// Datastar treats JSON responses as signal patches and consumes the body
	// before completing its request. A 204 response is valid HTTP, but Datastar
	// closes that branch by aborting its fetch controller, which browsers expose
	// as a failed request. The empty patch acknowledges command acceptance while
	// progressive results continue exclusively on the page /updates stream.
	writeJSON(w, nethttp.StatusOK, map[string]any{})
}

func streamPreparation(prepared command.PreparedRefresh) dashboardstream.RefreshPreparation {
	targets := make([]string, 0, len(prepared.Plan.Targets))
	for _, target := range prepared.Plan.Targets {
		targets = append(targets, string(target.Kind)+":"+target.ID)
	}
	return dashboardstream.RefreshPreparation{
		Filters: prepared.Filters,
		Command: prepared.Plan.Command,
		Targets: targets,
		Plan:    prepared.Plan,
	}
}

func (h Handler) readSignals(w nethttp.ResponseWriter, r *nethttp.Request) (dashboard.Signals, bool) {
	signals := dashboard.Signals{}
	if err := pagestream.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return dashboard.Signals{}, false
	}
	return signals, true
}
