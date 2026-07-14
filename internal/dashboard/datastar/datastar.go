package datastar

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	dashboardstream "github.com/Yacobolo/libredash/internal/dashboard/stream"
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
	return pagestream.SignalPatch{
		"filters":       patch.Filters,
		"filterOptions": patch.FilterOptions,
		"status":        patch.Status,
		"visuals":       patch.Visuals,
	}
}

func TablePatch(name string, table dashboard.Table) pagestream.SignalPatch {
	return pagestream.SignalPatch{
		"tables": map[string]dashboard.Table{
			name: table,
		},
	}
}

func TablesPatch(tables map[string]dashboard.Table) pagestream.SignalPatch {
	return pagestream.SignalPatch{"tables": tables}
}

func LoadingPatch(dataDir string) pagestream.SignalPatch {
	return pagestream.SignalPatch{
		"status": map[string]any{
			"loading":       true,
			"error":         "",
			"refreshId":     "",
			"generation":    int64(0),
			"lastUpdated":   "",
			"dataDirectory": dataDir,
			"setupRequired": false,
		},
	}
}

func RefreshEventPatch(event dashboardstream.RefreshEvent, dataDir string) pagestream.SignalPatch {
	generation := int64(event.Generation)
	status := func(loading bool, err error) map[string]any {
		message := ""
		if err != nil {
			message = err.Error()
		}
		return map[string]any{
			"loading":       loading,
			"error":         message,
			"refreshId":     event.RefreshID,
			"generation":    generation,
			"lastUpdated":   time.Now().Format("15:04:05"),
			"dataDirectory": dataDir,
			"setupRequired": errorSetupRequired(err),
		}
	}
	component := func(loading bool, err string) map[string]any {
		return map[string]any{"generation": generation, "loading": loading, "error": err}
	}
	switch event.Type {
	case dashboardstream.RefreshEventStart:
		components := map[string]any{}
		for _, target := range event.Targets {
			if strings.HasPrefix(target, "visual:") || strings.HasPrefix(target, "table:") {
				components[target] = component(true, "")
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
	case dashboardstream.RefreshEventVisual:
		visual, _ := event.Value.(dashboard.Visual)
		key := "visual:" + event.Target
		return pagestream.SignalPatch{
			"visuals":         map[string]dashboard.Visual{event.Target: visual},
			"componentStatus": map[string]any{key: component(false, "")},
		}
	case dashboardstream.RefreshEventTable:
		table, _ := event.Value.(dashboard.Table)
		key := "table:" + event.Target
		return pagestream.SignalPatch{
			"tables":          map[string]dashboard.Table{event.Target: table},
			"componentStatus": map[string]any{key: component(false, "")},
		}
	case dashboardstream.RefreshEventTargetError:
		if event.Target == "refresh" {
			return pagestream.SignalPatch{"status": status(false, event.Err)}
		}
		message := ""
		if event.Err != nil {
			message = event.Err.Error()
		}
		return pagestream.SignalPatch{"componentStatus": map[string]any{event.Target: component(false, message)}}
	case dashboardstream.RefreshEventComplete:
		return pagestream.SignalPatch{"status": status(false, event.Err)}
	default:
		return pagestream.SignalPatch{}
	}
}

func errorSetupRequired(err error) bool {
	var setup interface{ SetupRequired() bool }
	return errors.As(err, &setup) && setup.SetupRequired()
}
