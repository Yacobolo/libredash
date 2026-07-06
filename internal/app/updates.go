package app

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/ui"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
	"github.com/Yacobolo/libredash/pkg/pagestream"
)

type updateRouteSignals struct {
	Runtime uisignals.RouteRuntimeSignal `json:"runtime"`
}

func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	route := updateRoute(r)
	if route == "" {
		http.Error(w, "updates route is required", http.StatusBadRequest)
		return
	}
	permission, ok := updatesPermission(route, r.URL.Query().Get("section"))
	if !ok {
		http.Error(w, "unknown updates route", http.StatusBadRequest)
		return
	}
	if permission == "" {
		s.serveUpdates(w, r, route)
		return
	}
	s.protect(permission, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveUpdates(w, r, route)
	})).ServeHTTP(w, r)
}

func updateRoute(r *http.Request) string {
	route := strings.TrimSpace(r.URL.Query().Get("route"))
	if route != "" {
		return route
	}
	if strings.TrimSpace(r.URL.Query().Get("dashboard")) != "" {
		return string(uisignals.RouteDashboard)
	}
	var signals updateRouteSignals
	if err := pagestream.ReadSignals(r, &signals); err == nil {
		if signals.Runtime.RouteKey != "" {
			return string(signals.Runtime.RouteKey)
		}
		if signals.Runtime.Kind != "" {
			return string(signals.Runtime.Kind)
		}
	}
	return ""
}

func updatesPermission(route, section string) (string, bool) {
	switch uisignals.RouteKind(route) {
	case uisignals.RouteLogin:
		return "", true
	case uisignals.RouteCatalog, uisignals.RouteDashboard, uisignals.RouteWorkspace, uisignals.RouteWorkspaceAsset, uisignals.RouteConnections, uisignals.RouteConnectionAsset, uisignals.RouteData:
		return access.PermissionDashboardView, true
	case uisignals.RouteChat:
		return access.PermissionAgentRead, true
	case uisignals.RouteAdmin:
		if strings.TrimSpace(section) == "queries" {
			return access.PermissionAuditRead, true
		}
		return access.PermissionRBACRead, true
	default:
		return "", false
	}
}

func (s *Server) serveUpdates(w http.ResponseWriter, r *http.Request, route string) {
	switch uisignals.RouteKind(route) {
	case uisignals.RouteDashboard:
		s.dashboardHTTP().Updates(w, r)
	case uisignals.RouteChat:
		if s.agent == nil || !s.agent.Enabled() {
			s.chatBootstrapUpdates(w, r)
			return
		}
		s.chatUpdates(w, r)
	case uisignals.RouteData:
		s.dataExplorerUpdates(w, r)
	case uisignals.RouteAdmin:
		switch strings.TrimSpace(r.URL.Query().Get("section")) {
		case "queries":
			s.adminQueryHistoryUpdates(w, r)
		case "storage":
			s.adminStorageUpdates(w, r)
		default:
			s.adminBootstrapUpdates(w, r)
		}
	case uisignals.RouteWorkspaceAsset, uisignals.RouteConnectionAsset:
		if strings.TrimSpace(r.URL.Query().Get("asset")) != "" {
			s.workspaceAssetUpdates(w, r)
			return
		}
		s.patchAndWait(w, r, pagestream.SignalPatch{"status": map[string]any{"loading": false, "error": ""}})
	case uisignals.RouteLogin:
		s.patchAndWait(w, r, ui.LoginBootstrapSignals())
	case uisignals.RouteCatalog:
		s.patchAndWait(w, r, ui.CatalogBootstrapSignalsForCatalogs(s.catalogsForVisibleWorkspaces(r), s.chatChromeOption(r)))
	case uisignals.RouteWorkspace:
		s.workspaceBootstrapUpdates(w, r)
	case uisignals.RouteConnections:
		s.connectionsBootstrapUpdates(w, r)
	default:
		http.Error(w, "unknown updates route", http.StatusBadRequest)
	}
}

func (s *Server) noopUpdates(w http.ResponseWriter, r *http.Request) {
	s.patchAndWait(w, r, pagestream.SignalPatch{"status": map[string]any{"loading": false, "error": ""}})
}

func (s *Server) patchAndWait(w http.ResponseWriter, r *http.Request, patch pagestream.SignalPatch) {
	_ = pagestream.EnsureClientID(w, r)
	updates := pagestream.NewSignalStream(w, r)
	if err := updates.Patch(patch); err != nil {
		return
	}
	updates.Wait(r.Context())
}
