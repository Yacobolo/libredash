package app

import (
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
		r.Get("/workspaces", s.protected(access.PermissionDashboardView, s.workspaces))
		r.Get("/workspaces/{workspace}", s.protected(access.PermissionDashboardView, s.workspaceAssets))
		r.Get("/workspaces/{workspace}/assets/{asset}", s.protected(access.PermissionDashboardView, s.workspaceAsset))
		r.Get("/workspaces/{workspace}/assets/{asset}/{section}", s.protected(access.PermissionDashboardView, s.workspaceAssetSection))
		r.Get("/chat", s.protected(access.PermissionDashboardView, s.chat))
		r.Get("/chat/new", s.protected(access.PermissionDashboardView, s.chatNew))
		r.Get("/chat/updates", s.protected(access.PermissionDashboardView, s.chatUpdates))
		r.Get("/chat/{conversation}", s.protected(access.PermissionDashboardView, s.chatConversation))
		r.Post("/chat/turns", s.protected(access.PermissionDashboardView, s.chatTurn))
		r.Get("/admin", s.protected(access.PermissionRBACRead, s.adminGeneral))
		r.Get("/admin/principals", s.protected(access.PermissionRBACRead, s.adminPrincipals))
		r.Get("/admin/principals/{principal}", s.protected(access.PermissionRBACRead, s.adminPrincipalDetail))
		r.Get("/admin/groups", s.protected(access.PermissionRBACRead, s.adminGroups))
		r.Get("/admin/groups/{group}", s.protected(access.PermissionRBACRead, s.adminGroupDetail))
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
		r.Get("/dashboards/{dashboard}", s.protected(access.PermissionDashboardView, dashboardHTTP.Dashboard))
		r.Get("/dashboards/{dashboard}/pages/{page}", s.protected(access.PermissionDashboardView, dashboardHTTP.Page))
		r.With(s.rateLimits.updatesMiddleware()).Get("/updates", s.protected(access.PermissionDashboardView, dashboardHTTP.Updates))
		r.Post("/commands/table-window", s.protected(access.PermissionDashboardView, dashboardHTTP.TableWindow))
		r.Post("/commands/select", s.protected(access.PermissionDashboardView, dashboardHTTP.Select))
		r.Post("/commands/clear-selection", s.protected(access.PermissionDashboardView, dashboardHTTP.ClearSelection))
		r.Post("/commands/reset-filters", s.protected(access.PermissionDashboardView, dashboardHTTP.ResetFilters))
		r.Post("/commands/refresh-materializations", s.protected(access.PermissionMaterializationsRefresh, dashboardHTTP.RefreshMaterializations))
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
		return next
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
