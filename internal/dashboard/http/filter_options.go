package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"

	lddatastar "github.com/Yacobolo/leapview/internal/dashboard/datastar"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	dashboardsession "github.com/Yacobolo/leapview/internal/dashboard/session"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

type compiledFilterOptionMetrics interface {
	QueryCompiledFilterOptions(context.Context, string, dashboardfilter.OptionQuery) (dashboardfilter.OptionResult, error)
}

func (h Handler) FilterOptions(w nethttp.ResponseWriter, r *nethttp.Request) {
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
	definition, _, ok := metrics.Report(dashboardID)
	if !ok {
		nethttp.NotFound(w, r)
		return
	}
	request := signals.FilterOptionRequest
	bindings := definition.CompiledFilterBindings()
	binding, ok := bindings[request.BindingKey]
	if !ok || binding.Scope == dashboardfilter.ScopePage && binding.PageID != pageID {
		nethttp.Error(w, "unknown filter option binding", nethttp.StatusBadRequest)
		return
	}
	filterDefinition, ok := definition.FilterDefinitions[binding.Filter]
	if !ok {
		nethttp.Error(w, "unknown compiled filter definition", nethttp.StatusInternalServerError)
		return
	}
	if h.SessionStore == nil {
		nethttp.Error(w, "dashboard session store is unavailable", nethttp.StatusServiceUnavailable)
		return
	}
	clientID := pagestream.ClientIDFromRequest(r, signals.Runtime.ClientID)
	key := h.dashboardSessionKey(r, definition, clientID, signals.Runtime.StreamInstanceID)
	record, err := h.SessionStore.Load(r.Context(), key)
	if err != nil {
		status := nethttp.StatusServiceUnavailable
		if errors.Is(err, dashboardsession.ErrNotFound) {
			status = nethttp.StatusConflict
		}
		nethttp.Error(w, "dashboard session is unavailable", status)
		return
	}
	queryMetrics, supportsDynamicOptions := metrics.(compiledFilterOptionMetrics)
	engine := dashboardfilter.NewOptionEngineWithCache(h.OptionCursorSecret, h.OptionCache, func(ctx context.Context, query dashboardfilter.OptionQuery) (dashboardfilter.OptionResult, error) {
		if !supportsDynamicOptions {
			return dashboardfilter.OptionResult{}, fmt.Errorf("compiled filter options are not supported by this runtime")
		}
		return queryMetrics.QueryCompiledFilterOptions(ctx, dashboardID, query)
	})
	keysByRef := make(map[dashboardfilter.BindingRef]string, len(definition.FilterBindings)+len(bindings))
	for key, candidate := range bindings {
		if candidate.Scope == dashboardfilter.ScopePage && candidate.PageID != pageID {
			continue
		}
		keysByRef[dashboardfilter.BindingRef{Scope: candidate.Scope, ID: candidate.ID}] = key
	}
	policyIdentity := key.PrincipalOrClient
	page, err := engine.Page(r.Context(), dashboardfilter.OptionContext{
		ServingStateID: key.ServingStateID, PolicyIdentity: policyIdentity,
		State: record.State.Filters.State, Binding: binding, Definition: filterDefinition,
		BindingKeysByRef: keysByRef,
	}, request)
	if err != nil {
		writeFilterOptionError(w, err)
		return
	}
	page.StreamGeneration = record.State.StreamGeneration
	writeJSON(w, nethttp.StatusOK, map[string]any{
		"filterOptionPages": map[string]any{binding.Key: page},
	})
}

func writeFilterOptionError(w nethttp.ResponseWriter, err error) {
	if errors.Is(err, dashboardfilter.ErrStaleOptionRequest) {
		writeJSON(w, nethttp.StatusOK, map[string]any{"filterOptionPages": map[string]any{}})
		return
	}
	nethttp.Error(w, err.Error(), nethttp.StatusConflict)
}
