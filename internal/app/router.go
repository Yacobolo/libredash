package app

import (
	"context"
	"net/http"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/go-chi/chi/v5"
)

func (s *Server) Routes() http.Handler {
	mux := chi.NewRouter()
	if s.requestLogging {
		mux.Use(requestLogger(s.logger))
	}
	mux.Use(securityHeaders(s.securityHeaders))
	mux.Get("/favicon.ico", favicon)
	mux.Get("/login", s.login)
	mux.Group(func(r chi.Router) {
		r.Use(s.csrf)
		r.Get("/", s.protected(access.PermissionDashboardView, s.home))
		r.Get("/data", s.protected(access.PermissionDashboardView, s.dataExplorer))
		r.Get("/data/updates", s.protected(access.PermissionDashboardView, s.dataExplorerUpdates))
		r.Post("/data/command", s.protected(access.PermissionDashboardView, s.dataExplorerCommand))
		r.Get("/workspaces", s.protected(access.PermissionDashboardView, s.workspaces))
		r.Get("/workspaces/{workspace}", s.protected(access.PermissionDashboardView, s.workspaceAssets))
		r.Get("/workspaces/{workspace}/assets/{asset}", s.protected(access.PermissionDashboardView, s.workspaceAsset))
		r.Get("/workspaces/{workspace}/assets/{asset}/updates", s.protected(access.PermissionDashboardView, s.workspaceAssetUpdates))
		r.Get("/workspaces/{workspace}/assets/{asset}/{section}", s.protected(access.PermissionDashboardView, s.workspaceAssetSection))
		r.Post("/workspaces/{workspace}/assets/{asset}/refresh", s.protected(access.PermissionMaterializationsRefresh, s.refreshWorkspaceAsset))
		r.Post("/workspaces/{workspace}/assets/{asset}/refresh-materializations", s.protected(access.PermissionMaterializationsRefresh, s.refreshWorkspaceAssetMaterializations))
		r.Get("/workspaces/{workspace}/data", s.protected(access.PermissionDashboardView, s.workspaceDataExplorerRedirect))
		r.Get("/chat", s.protected(access.PermissionAgentRead, s.chat))
		r.Get("/chat/new", s.protected(access.PermissionAgentRead, s.chatNew))
		r.Get("/chat/updates", s.protected(access.PermissionAgentRead, s.chatUpdates))
		r.Get("/chat/{conversation}", s.protected(access.PermissionAgentRead, s.chatConversation))
		r.Post("/chat/turns", s.protected(access.PermissionAgentUse, s.chatTurn))
		r.Get("/workspaces/{workspace}/chat", s.protected(access.PermissionDashboardView, s.legacyChatRedirect))
		r.Get("/workspaces/{workspace}/chat/new", s.protected(access.PermissionDashboardView, s.legacyChatRedirect))
		r.Get("/workspaces/{workspace}/chat/updates", s.protected(access.PermissionDashboardView, s.legacyChatRedirect))
		r.Get("/workspaces/{workspace}/chat/{conversation}", s.protected(access.PermissionDashboardView, s.legacyChatRedirect))
		r.Post("/workspaces/{workspace}/chat/turns", s.protected(access.PermissionDashboardView, s.legacyChatRedirect))
		r.Get("/admin", s.protected(access.PermissionRBACRead, s.adminGeneral))
		r.Get("/admin/principals", s.protected(access.PermissionRBACRead, s.adminPrincipals))
		r.Get("/admin/principals/{principal}", s.protected(access.PermissionRBACRead, s.adminPrincipalDetail))
		r.Get("/admin/groups", s.protected(access.PermissionRBACRead, s.adminGroups))
		r.Get("/admin/groups/{group}", s.protected(access.PermissionRBACRead, s.adminGroupDetail))
		r.Get("/admin/agent", s.protected(access.PermissionRBACRead, s.adminAgent))
		r.Get("/admin/storage", s.protected(access.PermissionRBACRead, s.adminStorage))
		r.Get("/admin/storage/updates", s.protected(access.PermissionRBACRead, s.adminStorageUpdates))
		r.Post("/admin/storage/select-table", s.protected(access.PermissionRBACRead, s.adminStorageSelectTable))
		r.Get("/admin/queries", s.protected(access.PermissionAuditRead, s.adminQueries))
		r.Get("/admin/queries/updates", s.protected(access.PermissionAuditRead, s.adminQueryHistoryUpdates))
		r.Post("/admin/queries/command", s.protected(access.PermissionAuditRead, s.adminQueryHistoryCommand))
		r.Post("/workspaces/{workspace}/access/upsert", s.protected(access.PermissionRBACManage, s.upsertWorkspaceAccess))
		r.Post("/workspaces/{workspace}/access/remove", s.protected(access.PermissionRBACManage, s.removeWorkspaceAccess))
		r.Get("/workspaces/{workspace}/permissions", s.protected(access.PermissionRBACManage, s.workspacePermissions))
		r.Post("/workspaces/{workspace}/permissions", s.protected(access.PermissionRBACManage, s.updateWorkspacePermission))
		r.Post("/workspaces/{workspace}/permissions/remove", s.protected(access.PermissionRBACManage, s.removeWorkspacePermission))
		r.Get("/connections", s.protected(access.PermissionDashboardView, s.connections))
		r.Get("/connections/{connection}/sources/{source}", s.protected(access.PermissionDashboardView, s.connectionSourceAsset))
		r.Get("/connections/{connection}/sources/{source}/{section}", s.protected(access.PermissionDashboardView, s.connectionSourceAssetSection))
		r.Get("/connections/{asset}", s.protected(access.PermissionDashboardView, s.connectionAsset))
		r.Get("/connections/{asset}/{section}", s.protected(access.PermissionDashboardView, s.connectionAssetSection))
		dashboardHTTP := s.dashboardHTTP()
		r.Get("/workspaces/{workspace}/dashboards/{dashboard}", s.protected(access.PermissionDashboardView, dashboardHTTP.Dashboard))
		r.Get("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}", s.protected(access.PermissionDashboardView, dashboardHTTP.Page))
		r.With(s.rateLimits.updatesMiddleware()).Get("/workspaces/{workspace}/updates", s.protected(access.PermissionDashboardView, dashboardHTTP.Updates))
		r.Post("/workspaces/{workspace}/commands/table-window", s.protected(access.PermissionDashboardView, dashboardHTTP.TableWindow))
		r.Post("/workspaces/{workspace}/commands/select", s.protected(access.PermissionDashboardView, dashboardHTTP.Select))
		r.Post("/workspaces/{workspace}/commands/clear-selection", s.protected(access.PermissionDashboardView, dashboardHTTP.ClearSelection))
		r.Post("/workspaces/{workspace}/commands/reset-filters", s.protected(access.PermissionDashboardView, dashboardHTTP.ResetFilters))
		r.Post("/workspaces/{workspace}/commands/refresh-materializations", s.protected(access.PermissionMaterializationsRefresh, dashboardHTTP.RefreshMaterializations))
		r.Post("/auth/logout", s.authLogout)
	})
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.authMiddleware())
		r.Get("/auth/{provider}", s.authBegin)
		r.Get("/auth/{provider}/callback", s.authCallback)
	})
	if s.store != nil {
		mux.Group(func(r chi.Router) {
			r.Use(s.rateLimits.apiMiddleware())
			r.Use(s.csrf)
			r.Get("/api/v1/agent/conversations", s.protected(access.PermissionAgentRead, s.listAgentConversations))
			r.Post("/api/v1/agent/conversations", s.protected(access.PermissionAgentUse, s.createAgentConversation))
			r.Get("/api/v1/agent/conversations/{conversation}", s.protected(access.PermissionAgentRead, s.getAgentConversation))
			r.Patch("/api/v1/agent/conversations/{conversation}", s.protected(access.PermissionAgentUse, s.updateAgentConversation))
			r.Delete("/api/v1/agent/conversations/{conversation}", s.protected(access.PermissionAgentUse, s.archiveAgentConversation))
			r.Get("/api/v1/agent/conversations/{conversation}/messages", s.protected(access.PermissionAgentRead, s.listAgentMessages))
			r.Get("/api/v1/agent/conversations/{conversation}/runs", s.protected(access.PermissionAgentRead, s.listAgentRuns))
			r.Get("/api/v1/agent/conversations/{conversation}/runs/{run}", s.protected(access.PermissionAgentRead, s.getAgentRun))
			r.Get("/api/v1/agent/conversations/{conversation}/runs/{run}/events", s.protected(access.PermissionAgentRead, s.listAgentEvents))
			r.Post("/api/v1/agent/conversations/{conversation}/turns", s.protected(access.PermissionAgentUse, s.createAgentTurn))
			s.registerAPIGenRoutes(r)
		})
	}
	mux.Handle("/static/*", noCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

	return mux
}

func (s *Server) protected(permission string, handler http.HandlerFunc) http.HandlerFunc {
	return s.protect(permission, handler).ServeHTTP
}

func (s *Server) protect(permission string, next http.Handler) http.Handler {
	if s.auth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), principalContextKey{}, localDeveloperPrincipal())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return s.auth.Middleware(permission, next)
}

func (s *Server) csrf(next http.Handler) http.Handler {
	if s.auth == nil {
		return next
	}
	return s.auth.CSRFMiddleware(next)
}

func (s *Server) authBegin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.Begin(w, r)
}

func (s *Server) authCallback(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.Callback(w, r)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.Logout(w, r)
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="6" fill="#0969da"/><path d="M8 22h16v3H8zm1-5h4v4H9zm5-7h4v11h-4zm5 4h4v7h-4z" fill="#fff"/></svg>`))
}
