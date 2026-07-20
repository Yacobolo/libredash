package app

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func (s *Server) pageStream(w http.ResponseWriter, r *http.Request) {
	route := streamRoute(r)
	if route == "" {
		http.Error(w, "updates route is required", http.StatusBadRequest)
		return
	}
	privilege, ok := streamPrivilege(route, r.URL.Query().Get("section"))
	if !ok {
		http.Error(w, "unknown updates route", http.StatusBadRequest)
		return
	}
	if privilege == "" {
		s.servePageStream(w, r, route)
		return
	}
	s.protect(privilege, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.servePageStream(w, r, route)
	})).ServeHTTP(w, r)
}

func streamRoute(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("route"))
}

func streamPrivilege(route, section string) (access.Privilege, bool) {
	switch uisignals.RouteKind(route) {
	case uisignals.RouteLogin:
		return "", true
	case uisignals.RouteCatalog, uisignals.RouteDashboard, uisignals.RouteWorkspace, uisignals.RouteWorkspaceAsset, uisignals.RouteConnections, uisignals.RouteConnectionAsset, uisignals.RouteData:
		return access.PrivilegeViewItem, true
	case uisignals.RouteChat:
		return access.PrivilegeViewAgent, true
	case uisignals.RouteAdmin:
		if strings.TrimSpace(section) == "queries" {
			return access.PrivilegeViewAudit, true
		}
		return access.PrivilegeManageGrants, true
	default:
		return "", false
	}
}

func (s *Server) servePageStream(w http.ResponseWriter, r *http.Request, route string) {
	switch uisignals.RouteKind(route) {
	case uisignals.RouteDashboard:
		s.dashboardHTTP().Updates(w, r)
	case uisignals.RouteChat:
		s.agentHTTPHandler().ChatUpdates(w, r)
	case uisignals.RouteData:
		s.workspaceHTTPHandler().DataExplorerUpdates(w, r)
	case uisignals.RouteAdmin:
		adminHTTP := s.adminHTTPHandler()
		switch strings.TrimSpace(r.URL.Query().Get("section")) {
		case "queries":
			adminHTTP.QueryUpdates(w, r)
		case "storage":
			adminHTTP.StorageSignalUpdates(w, r)
		default:
			adminHTTP.BootstrapUpdates(w, r)
		}
	case uisignals.RouteWorkspaceAsset, uisignals.RouteConnectionAsset:
		if strings.TrimSpace(r.URL.Query().Get("asset")) != "" {
			s.workspaceHTTPHandler().AssetUpdatesStream(w, r)
			return
		}
		s.patchAndWait(w, r, pagestream.SignalPatch{"status": map[string]any{"loading": false, "error": ""}})
	case uisignals.RouteLogin:
		s.patchAndWait(w, r, ui.LoginBootstrapSignalsForOptions(s.loginPageOptions(r)))
	case uisignals.RouteCatalog:
		s.patchAndWait(w, r, ui.CatalogBootstrapSignalsForCatalogs(s.workspaceHTTPReadModel().CatalogsForVisibleWorkspaces(r), s.chatChromeOption(r)))
	case uisignals.RouteWorkspace:
		s.workspaceHTTPHandler().WorkspaceBootstrapUpdates(w, r)
	case uisignals.RouteConnections:
		s.workspaceHTTPHandler().ConnectionsBootstrapUpdates(w, r)
	default:
		http.Error(w, "unknown updates route", http.StatusBadRequest)
	}
}

func (s *Server) noopUpdates(w http.ResponseWriter, r *http.Request) {
	s.patchAndWait(w, r, pagestream.SignalPatch{"status": map[string]any{"loading": false, "error": ""}})
}

func (s *Server) patchAndWait(w http.ResponseWriter, r *http.Request, patch pagestream.SignalPatch) {
	clientID := pagestream.EnsureClientID(w, r)
	streamID := streamRoute(r) + ":" + clientID
	updates := pagestream.NewSignalStream(w, r, pagestream.WithStreamTrace(s.pageStreamTrace, streamID, streamRoute(r)+".bootstrap"))
	if err := updates.Patch(patch); err != nil {
		return
	}
	updates.Wait(r.Context())
}
