package http

import (
	nethttp "net/http"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/command"
	lddatastar "github.com/Yacobolo/libredash/internal/dashboard/datastar"
)

func (h Handler) TableWindow(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(ctx command.Service, request command.Request) []command.Event {
		return ctx.TableWindow(r.Context(), request)
	})
}

func (h Handler) Select(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(ctx command.Service, request command.Request) []command.Event {
		return ctx.Select(r.Context(), request)
	})
}

func (h Handler) ClearSelection(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(ctx command.Service, request command.Request) []command.Event {
		return ctx.ClearSelection(r.Context(), request)
	})
}

func (h Handler) ResetFilters(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(ctx command.Service, request command.Request) []command.Event {
		return ctx.ResetFilters(r.Context(), request)
	})
}

func (h Handler) RefreshMaterializations(w nethttp.ResponseWriter, r *nethttp.Request) {
	h.handleCommand(w, r, func(ctx command.Service, request command.Request) []command.Event {
		return ctx.RefreshMaterializations(r.Context(), request)
	})
}

func (h Handler) handleCommand(w nethttp.ResponseWriter, r *nethttp.Request, run func(command.Service, command.Request) []command.Event) {
	signals, ok := h.readSignals(w, r)
	if !ok {
		return
	}
	dashboardID := lddatastar.DashboardID(r, signals, h.Metrics.DefaultDashboardID())
	pageID := lddatastar.PageID(r, signals)
	modelID := lddatastar.ModelID(r, signals, dashboardID, h.Metrics.ModelIDForDashboard)
	clientID := lddatastar.ClientStreamID(r, signals, dashboardID, pageID)

	events := run(command.Service{Metrics: h.Metrics}, command.Request{
		DashboardID:        dashboardID,
		PageID:             pageID,
		ModelID:            modelID,
		Filters:            signals.Filters,
		TableCommand:       signals.TableCommand,
		InteractionCommand: signals.InteractionCommand,
	})
	for _, event := range events {
		h.Broker.Publish(clientID, lddatastar.CommandEventPatch(event))
	}
	w.WriteHeader(nethttp.StatusNoContent)
}

func (h Handler) readSignals(w nethttp.ResponseWriter, r *nethttp.Request) (dashboard.Signals, bool) {
	signals := dashboard.Signals{}
	if err := lddatastar.ReadSignals(r, &signals); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return dashboard.Signals{}, false
	}
	return signals, true
}
