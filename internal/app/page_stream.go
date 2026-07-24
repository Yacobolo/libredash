package app

import (
	"net/http"
	"strings"

	"github.com/Yacobolo/leapview/internal/ui"
	uisignals "github.com/Yacobolo/leapview/internal/ui/signals"
	uitransport "github.com/Yacobolo/leapview/internal/ui/transport"
	"github.com/Yacobolo/leapview/pkg/pagestream"
)

func configurePageStream(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) {
	runtime.pageStreams = uitransport.NewPageStream(uitransport.PageStreamConfig{
		Trace: runtime.pageStreamTrace,
		Protect: func(privilege string, next http.Handler) http.Handler {
			return routes.accessModule.ProtectNamed(privilege, next)
		},
		ProtectGlobal: func(privilege string, next http.Handler) http.Handler {
			return routes.accessModule.ProtectGlobalNamed(privilege, next)
		},
		ProtectAnyWorkspace: func(privilege string, next http.Handler) http.Handler {
			return routes.accessModule.ProtectAnyWorkspaceNamed(privilege, next)
		},
		Handlers: map[uisignals.RouteKind]http.Handler{
			uisignals.RouteDashboard: http.HandlerFunc(routes.dashboardModule.HTTP().Updates),
			uisignals.RouteChat:      http.HandlerFunc(routes.agentModule.HTTP().ChatUpdates),
			uisignals.RouteData:      http.HandlerFunc(routes.workspaceModule.HTTP().DataExplorerUpdates),
			uisignals.RouteAdmin: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				adminHTTP := routes.adminModule.HTTP()
				switch strings.TrimSpace(r.URL.Query().Get("section")) {
				case "queries":
					adminHTTP.QueryUpdates(w, r)
				case "storage":
					adminHTTP.StorageSignalUpdates(w, r)
				default:
					adminHTTP.BootstrapUpdates(w, r)
				}
			}),
			uisignals.RouteWorkspaceAsset: http.HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				workspaceAssetUpdates(routes, runtime, platform, policy, w, r)
			})),
			uisignals.RouteConnectionAsset: http.HandlerFunc(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				workspaceAssetUpdates(routes, runtime, platform, policy, w, r)
			})),
			uisignals.RouteLogin: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				uitransport.PatchAndWait(runtime.pageStreamTrace, w, r, ui.LoginBootstrapSignalsForOptions(routes.accessModule.LoginPageOptions(r)))
			}),
			uisignals.RouteCatalog: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				uitransport.PatchAndWait(runtime.pageStreamTrace, w, r, ui.CatalogBootstrapSignalsForCatalogs(
					routes.workspaceModule.CatalogsForVisibleWorkspaces(r), routes.agentModule.ChromeOption(r),
				))
			}),
			uisignals.RouteWorkspace:   http.HandlerFunc(routes.workspaceModule.HTTP().WorkspaceBootstrapUpdates),
			uisignals.RouteConnections: http.HandlerFunc(routes.workspaceModule.HTTP().ConnectionsBootstrapUpdates),
		},
	})
}

func workspaceAssetUpdates(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.URL.Query().Get("asset")) != "" {
		routes.workspaceModule.HTTP().AssetUpdatesStream(w, r)
		return
	}
	uitransport.PatchAndWait(runtime.pageStreamTrace, w, r, pagestream.SignalPatch{"status": map[string]any{"loading": false, "error": ""}})
}
