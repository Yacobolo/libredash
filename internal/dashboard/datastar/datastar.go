package datastar

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

func DashboardID(r *http.Request, signals dashboard.Signals, defaultID string) string {
	if id := r.URL.Query().Get("dashboard"); id != "" {
		return id
	}
	if signals.Runtime.DashboardID != "" {
		return signals.Runtime.DashboardID
	}
	return defaultID
}

func PageID(r *http.Request, signals dashboard.Signals) string {
	if id := r.URL.Query().Get("page"); id != "" {
		return id
	}
	if signals.Runtime.PageID != "" {
		return signals.Runtime.PageID
	}
	return ""
}

func ModelID(r *http.Request, signals dashboard.Signals, dashboardID string, defaultForDashboard func(string) string) string {
	if id := r.URL.Query().Get("model"); id != "" {
		return id
	}
	if signals.Runtime.ModelID != "" {
		return signals.Runtime.ModelID
	}
	return defaultForDashboard(dashboardID)
}

func ClientStreamID(r *http.Request, signals dashboard.Signals, dashboardID, pageID string) string {
	instanceID := signals.Runtime.StreamInstanceID
	if instanceID == "" {
		instanceID = r.URL.Query().Get("streamInstance")
	}
	return StreamID(pagestream.ClientIDFromRequest(r, signals.Runtime.ClientID), dashboardID, pageID, instanceID)
}

func StreamID(clientID, dashboardID, pageID string, streamInstanceID ...string) string {
	streamID := clientID + ":" + dashboardID + ":" + pageID
	if len(streamInstanceID) > 0 && streamInstanceID[0] != "" {
		streamID += ":" + streamInstanceID[0]
	}
	return streamID
}

func DashboardPatch(patch dashboard.Patch) pagestream.SignalPatch {
	patch.Status.ProgressPercent = dashboard.NormalizeProgressPercent(patch.Status.ProgressPercent, patch.Status.Loading)
	return pagestream.SignalPatch{
		"filters":       patch.Filters,
		"filterOptions": patch.FilterOptions,
		"status":        patch.Status,
		"visuals":       visualSignals(patch.Visuals),
	}
}

func TablePatch(name string, table dashboard.Table) pagestream.SignalPatch {
	return pagestream.SignalPatch{
		"visuals": map[string]dashboard.TabularVisual{
			name: dashboard.NewTabularVisual(name, table),
		},
	}
}

func TablesPatch(tables map[string]dashboard.Table) pagestream.SignalPatch {
	visuals := make(map[string]dashboard.TabularVisual, len(tables))
	for id, table := range tables {
		visuals[id] = dashboard.NewTabularVisual(id, table)
	}
	return pagestream.SignalPatch{"visuals": visuals}
}

func LoadingPatch() pagestream.SignalPatch {
	return pagestream.SignalPatch{
		"status": map[string]any{
			"loading":         true,
			"error":           "",
			"refreshId":       "",
			"generation":      int64(0),
			"lastUpdated":     "",
			"setupRequired":   false,
			"progressPercent": dashboard.NormalizeProgressPercent(nil, true),
		},
	}
}

func RefreshEventPatch(event dashboardstream.RefreshEvent) pagestream.SignalPatch {
	generation := int64(event.Generation)
	status := func(loading bool, err error) map[string]any {
		message := ""
		if err != nil {
			message = err.Error()
		}
		return map[string]any{
			"loading":         loading,
			"error":           message,
			"refreshId":       event.RefreshID,
			"generation":      generation,
			"setupRequired":   errorSetupRequired(err),
			"progressPercent": dashboard.NormalizeProgressPercent(event.ProgressPercent, loading),
		}
	}
	component := func(loading bool, err string) map[string]any {
		return map[string]any{"generation": generation, "loading": loading, "error": err}
	}
	switch event.Type {
	case dashboardstream.RefreshEventStart:
		components := map[string]any{}
		for _, target := range event.Targets {
			if strings.HasPrefix(target, "visual:") {
				components[visualStatusKey(target)] = component(true, "")
			}
		}
		return pagestream.SignalPatch{
			"filters":         event.Filters,
			"status":          status(true, nil),
			"componentStatus": components,
		}
	case dashboardstream.RefreshEventFilterOptions:
		options, _ := event.Value.(map[string][]dashboard.FilterOption)
		return pagestream.SignalPatch{"filterOptions": options}
	case dashboardstream.RefreshEventProgress:
		return pagestream.SignalPatch{"status": status(true, nil)}
	case dashboardstream.RefreshEventVisual:
		visual, _ := event.Value.(dashboard.Visual)
		key := "visual:" + event.Target
		return pagestream.SignalPatch{
			"visuals":         map[string]uisignals.DashboardVisual{event.Target: uisignals.DashboardVisualFromDashboard(visual)},
			"componentStatus": map[string]any{key: component(false, "")},
		}
	case dashboardstream.RefreshEventTable:
		table, _ := event.Value.(dashboard.Table)
		key := "visual:" + event.Target
		return pagestream.SignalPatch{
			"visuals":         map[string]dashboard.TabularVisual{event.Target: dashboard.NewTabularVisual(event.Target, table)},
			"componentStatus": map[string]any{key: component(false, "")},
		}
	case dashboardstream.RefreshEventTableMetadata:
		table, _ := event.Value.(dashboard.Table)
		return pagestream.SignalPatch{"visuals": map[string]dashboard.TabularVisual{event.Target: dashboard.NewTabularVisual(event.Target, table)}}
	case dashboardstream.RefreshEventTargetError:
		if event.Target == "refresh" {
			return pagestream.SignalPatch{"status": status(false, event.Err)}
		}
		message := ""
		if event.Err != nil {
			message = event.Err.Error()
		}
		return pagestream.SignalPatch{"componentStatus": map[string]any{visualStatusKey(event.Target): component(false, message)}}
	case dashboardstream.RefreshEventComplete:
		return pagestream.SignalPatch{"status": status(false, event.Err)}
	default:
		return pagestream.SignalPatch{}
	}
}

func visualSignals(values map[string]dashboard.Visual) map[string]uisignals.DashboardVisual {
	out := make(map[string]uisignals.DashboardVisual, len(values))
	for id, visual := range values {
		out[id] = uisignals.DashboardVisualFromDashboard(visual)
	}
	return out
}

// RefreshEventEnvelope keeps refresh ordering and mailbox behavior outside the
// signal payload. The browser receives Signals only; pagestream consumes the
// explicit delivery metadata.
func RefreshEventEnvelope(event dashboardstream.RefreshEvent) pagestream.Envelope {
	generation := uint64(0)
	if event.Generation > 0 {
		generation = uint64(event.Generation)
	}
	delivery := pagestream.DeliveryMetadata{Generation: generation}
	switch event.Type {
	case dashboardstream.RefreshEventStart, dashboardstream.RefreshEventProgress, dashboardstream.RefreshEventComplete:
		delivery.Boundary = true
	case dashboardstream.RefreshEventTargetError:
		if event.Target == "refresh" {
			delivery.Boundary = true
		} else {
			delivery.CoalesceGroup = "dashboard-results"
			delivery.MergeRoots = dashboardMergeRoots()
		}
	default:
		delivery.CoalesceGroup = "dashboard-results"
		delivery.MergeRoots = dashboardMergeRoots()
	}
	return pagestream.Envelope{
		Signals:  RefreshEventPatch(event),
		Delivery: delivery,
		Trace: pagestream.TraceMetadata{
			Origin:        "dashboard.refresh",
			CorrelationID: event.RefreshID,
		},
	}
}

func dashboardMergeRoots() []string {
	return []string{"componentStatus", "filterOptions", "visuals"}
}

func visualStatusKey(target string) string {
	return strings.TrimPrefix(target, "visual:visual:")
}

func errorSetupRequired(err error) bool {
	var setup interface{ SetupRequired() bool }
	return errors.As(err, &setup) && setup.SetupRequired()
}
