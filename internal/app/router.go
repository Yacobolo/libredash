package app

import (
	"net/http"

	"github.com/Yacobolo/libredash/internal/platform"
	"github.com/go-chi/chi/v5"
)

func (s *Server) Routes() http.Handler {
	mux := chi.NewRouter()
	if s.requestLogging {
		mux.Use(requestLogger(s.logger))
	}
	mux.Use(securityHeaders(s.securityHeaders))
	mux.Get("/login", s.login)
	mux.Group(func(r chi.Router) {
		r.Use(s.csrf)
		r.Get("/", s.protected(platform.PermissionDashboardView, s.home))
		r.Get("/dashboards/{dashboard}", s.protected(platform.PermissionDashboardView, s.dashboard))
		r.Get("/dashboards/{dashboard}/pages/{page}", s.protected(platform.PermissionDashboardView, s.page))
		r.Get("/metrics", s.protected(platform.PermissionDashboardView, s.metricsCatalog))
		r.Get("/metrics/{view}", s.protected(platform.PermissionDashboardView, s.metricView))
		r.Get("/metrics/{view}/{section}", s.protected(platform.PermissionDashboardView, s.metricViewSection))
		r.Get("/models", s.protected(platform.PermissionDashboardView, s.models))
		r.Get("/models/{model}", s.protected(platform.PermissionDashboardView, s.model))
		r.With(s.rateLimits.updatesMiddleware()).Get("/updates", s.protected(platform.PermissionDashboardView, s.updates))
		r.Post("/commands/table-window", s.protected(platform.PermissionDashboardView, s.tableWindow))
		r.Post("/commands/chart-select", s.protected(platform.PermissionDashboardView, s.chartSelect))
		r.Post("/commands/clear-selection", s.protected(platform.PermissionDashboardView, s.clearSelection))
		r.Post("/commands/reset-filters", s.protected(platform.PermissionDashboardView, s.resetFilters))
		r.Post("/commands/refresh-cache", s.protected(platform.PermissionCacheRefresh, s.refreshCache))
		r.Post("/auth/logout", s.authLogout)
	})
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.authMiddleware())
		r.Get("/auth/{provider}", s.authBegin)
		r.Get("/auth/{provider}/callback", s.authCallback)
	})
	if s.store != nil {
		mux.Route("/api", func(r chi.Router) {
			r.Use(s.rateLimits.apiMiddleware())
			r.Use(s.csrf)
			r.Post("/deployments", s.protected(platform.PermissionDeploymentCreate, s.createDeployment))
			r.Get("/deployments", s.protected(platform.PermissionDeploymentCreate, s.listDeployments))
			r.Get("/deployments/{deployment}", s.protected(platform.PermissionDeploymentCreate, s.getDeployment))
			r.Put("/deployments/{deployment}/artifact", s.protected(platform.PermissionDeploymentCreate, s.uploadDeploymentArtifact))
			r.Post("/deployments/{deployment}/validate", s.protected(platform.PermissionDeploymentCreate, s.validateDeployment))
			r.Post("/deployments/{deployment}/activate", s.protected(platform.PermissionDeploymentActivate, s.activateDeployment))
			r.Post("/deployments/{deployment}/rollback", s.protected(platform.PermissionDeploymentRollback, s.rollbackDeployment))
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
