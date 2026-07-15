package app

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/access/httpauth"
	"github.com/Yacobolo/libredash/internal/access/scimprov"
	agenthttp "github.com/Yacobolo/libredash/internal/agent/http"
	dashboardhttp "github.com/Yacobolo/libredash/internal/dashboard/http"
	"github.com/Yacobolo/libredash/internal/staticasset"
	workspacehttp "github.com/Yacobolo/libredash/internal/workspace/http"
	"github.com/go-chi/chi/v5"
)

func (s *Server) Routes() http.Handler {
	mux := chi.NewRouter()
	if s.requestLogging {
		mux.Use(requestLogger(s.logger))
	}
	mux.Use(s.telemetry.middleware)
	mux.Use(panicRecovery(s.logger))
	mux.Use(securityHeaders(s.securityHeaders))
	mux.Use(allowedHosts(s.allowedHosts))
	mux.Use(requestBodyLimit(s.requestBodyLimit))
	mux.Get("/favicon.ico", favicon)
	mux.Get("/healthz", s.healthz)
	mux.Get("/readyz", s.readyz)
	mux.With(s.rateLimits.authMiddleware()).Handle("/metrics", s.metricsHandler())
	mux.With(s.csrf).Get("/login", s.login)
	mux.Group(func(r chi.Router) {
		r.Use(s.csrf)
		r.With(s.rateLimits.updatesMiddleware()).Get("/updates", s.pageStream)
		r.Get("/", s.protected(access.PrivilegeViewItem, s.home))
		workspaceHTTP := s.workspaceHTTPHandler()
		r.Get("/data", s.protected(access.PrivilegeViewItem, workspaceHTTP.DataExplorer))
		r.Post("/data/command", s.protected(access.PrivilegeViewItem, workspaceHTTP.DataExplorerCommand))
		r.Get("/workspaces", s.protected(access.PrivilegeViewItem, workspaceHTTP.WorkspaceCatalog))
		r.Get("/workspaces/{workspace}", s.protected(access.PrivilegeViewItem, workspaceHTTP.WorkspaceAssets))
		r.Get("/workspaces/{workspace}/assets/{asset}", s.protectedWithObjects(access.PrivilegeViewItem, workspacehttp.AssetObjectRefs, workspaceHTTP.WorkspaceAsset))
		r.Get("/workspaces/{workspace}/assets/{asset}/{section}", s.protectedWithObjects(access.PrivilegeViewItem, workspacehttp.AssetObjectRefs, workspaceHTTP.WorkspaceAssetSection))
		r.Post("/workspaces/{workspace}/assets/{asset}/refresh", s.protectedWithObjects(access.PrivilegeRefreshData, workspacehttp.AssetObjectRefs, workspaceHTTP.RefreshAsset))
		r.Post("/workspaces/{workspace}/assets/{asset}/refresh-materializations", s.protectedWithObjects(access.PrivilegeRefreshData, workspacehttp.AssetObjectRefs, workspaceHTTP.RefreshAssetMaterializations))
		r.Get("/workspaces/{workspace}/data", s.protected(access.PrivilegeViewItem, workspaceHTTP.WorkspaceDataExplorerRedirect))
		agentHTTP := s.agentHTTPHandler()
		r.Get("/chat", s.protected(access.PrivilegeViewAgent, agentHTTP.Chat))
		r.Get("/chat/new", s.protected(access.PrivilegeViewAgent, agentHTTP.ChatNew))
		r.Get("/chat/{conversation}", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.ChatConversation))
		r.Post("/chat/turns", s.protected(access.PrivilegeUseAgent, agentHTTP.ChatTurn))
		adminHTTP := s.adminHTTPHandler()
		r.Get("/admin", s.protected(access.PrivilegeManageGrants, adminHTTP.General))
		r.Get("/admin/principals", s.protected(access.PrivilegeManageGrants, adminHTTP.Principals))
		r.Get("/admin/principals/{principal}", s.protected(access.PrivilegeManageGrants, adminHTTP.PrincipalDetail))
		r.Get("/admin/groups", s.protected(access.PrivilegeManageGrants, adminHTTP.Groups))
		r.Get("/admin/groups/{group}", s.protected(access.PrivilegeManageGrants, adminHTTP.GroupDetail))
		r.Get("/admin/agent", s.protected(access.PrivilegeManageGrants, adminHTTP.Agent))
		r.Get("/admin/storage", s.protected(access.PrivilegeManageGrants, adminHTTP.Storage))
		r.Post("/admin/storage/select-table", s.protected(access.PrivilegeManageGrants, adminHTTP.StorageTableSelect))
		r.Get("/admin/queries", s.protected(access.PrivilegeViewAudit, adminHTTP.Queries))
		r.Post("/admin/queries/command", s.protected(access.PrivilegeViewAudit, adminHTTP.QueryCommand))
		r.Post("/workspaces/{workspace}/access/upsert", s.protected(access.PrivilegeManageGrants, workspaceHTTP.AccessUpsert))
		r.Post("/workspaces/{workspace}/access/remove", s.protected(access.PrivilegeManageGrants, workspaceHTTP.AccessRemove))
		r.Post("/workspaces/{workspace}/assets/{asset}/access/upsert", s.protectedWithObjects(access.PrivilegeManageGrants, workspacehttp.AssetObjectRefs, workspaceHTTP.AccessUpsert))
		r.Post("/workspaces/{workspace}/assets/{asset}/access/remove", s.protectedWithObjects(access.PrivilegeManageGrants, workspacehttp.AssetObjectRefs, workspaceHTTP.AccessRemove))
		r.Get("/connections", s.protected(access.PrivilegeViewItem, workspaceHTTP.Connections))
		r.Get("/connections/{connection}/sources/{source}", s.protected(access.PrivilegeViewItem, workspaceHTTP.ConnectionSource))
		r.Get("/connections/{connection}/sources/{source}/{section}", s.protected(access.PrivilegeViewItem, workspaceHTTP.ConnectionSourceSection))
		r.Get("/connections/{asset}", s.protected(access.PrivilegeViewItem, workspaceHTTP.ConnectionAsset))
		r.Get("/connections/{asset}/{section}", s.protected(access.PrivilegeViewItem, workspaceHTTP.ConnectionAssetSection))
		dashboardHTTP := s.dashboardHTTP()
		r.Get("/workspaces/{workspace}/dashboards/{dashboard}", s.protectedWithObjects(access.PrivilegeViewItem, dashboardhttp.DashboardObjectRefs, dashboardHTTP.Dashboard))
		r.Get("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}", s.protectedWithObjects(access.PrivilegeViewItem, dashboardhttp.DashboardObjectRefs, dashboardHTTP.Page))
		r.Post("/workspaces/{workspace}/commands/table-window", s.protected(access.PrivilegeViewItem, dashboardHTTP.TableWindow))
		r.Post("/workspaces/{workspace}/commands/select", s.protected(access.PrivilegeViewItem, dashboardHTTP.Select))
		r.Post("/workspaces/{workspace}/commands/clear-selection", s.protected(access.PrivilegeViewItem, dashboardHTTP.ClearSelection))
		r.Post("/workspaces/{workspace}/commands/reload", s.protected(access.PrivilegeViewItem, dashboardHTTP.Reload))
		r.Post("/workspaces/{workspace}/commands/reset-filters", s.protected(access.PrivilegeViewItem, dashboardHTTP.ResetFilters))
		r.Post("/auth/logout", s.authLogout)
		r.Post("/auth/local/password", s.authLocalPassword)
	})
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.authMiddleware())
		r.Use(s.csrf)
		r.Post("/auth/local/login", s.authLocalLogin)
	})
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.authMiddleware())
		r.Get("/auth/{provider}", s.authBegin)
		r.Get("/auth/{provider}/callback", s.authCallback)
		r.Post("/oauth/token", s.accessHTTPHandler().OAuthToken)
	})
	if s.store != nil {
		if strings.TrimSpace(s.scimBearerToken) != "" {
			if repo, err := s.accessRepository(); err == nil && repo != nil {
				if handler, err := scimprov.NewHandler(scimprov.Options{Repository: repo, BearerToken: s.scimBearerToken}); err == nil {
					scimHandler := s.rateLimits.apiMiddleware()(http.StripPrefix("/scim", handler))
					mux.Handle("/scim/*", scimHandler)
				}
			}
		}
		mux.Group(func(r chi.Router) {
			r.Use(s.rateLimits.apiMiddleware())
			r.Use(s.csrf)
			if s.managedDataTus != nil {
				tus := s.protect(access.PrivilegeIngestData, managedDataTusHandler(s.managedDataTus))
				r.Handle("/api/v1/managed-data/tus", tus)
				r.Handle("/api/v1/managed-data/tus/*", tus)
			}
			agentHTTP := s.agentHTTPHandler()
			r.Get("/api/v1/agent/conversations", s.protected(access.PrivilegeViewAgent, agentHTTP.ListConversations))
			r.Post("/api/v1/agent/conversations", s.protected(access.PrivilegeUseAgent, agentHTTP.CreateConversation))
			r.Get("/api/v1/agent/conversations/{conversation}", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.GetConversation))
			r.Patch("/api/v1/agent/conversations/{conversation}", s.protectedWithObjects(access.PrivilegeUseAgent, agenthttp.ConversationObjectRefs, agentHTTP.UpdateConversation))
			r.Delete("/api/v1/agent/conversations/{conversation}", s.protectedWithObjects(access.PrivilegeUseAgent, agenthttp.ConversationObjectRefs, agentHTTP.ArchiveConversation))
			r.Get("/api/v1/agent/conversations/{conversation}/messages", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.ListMessages))
			r.Get("/api/v1/agent/conversations/{conversation}/runs", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.ListRuns))
			r.Get("/api/v1/agent/conversations/{conversation}/runs/{run}", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.GetRun))
			r.Get("/api/v1/agent/conversations/{conversation}/runs/{run}/events", s.protectedWithObjects(access.PrivilegeViewAgent, agenthttp.ConversationObjectRefs, agentHTTP.ListEvents))
			r.Post("/api/v1/agent/conversations/{conversation}/turns", s.protectedWithObjects(access.PrivilegeUseAgent, agenthttp.ConversationObjectRefs, agentHTTP.CreateTurn))
			s.registerAPIGenRoutes(r)
		})
	}
	mux.Handle("/static/*", staticAssetCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))

	return mux
}

func managedDataTusHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			// Authentication and headers have already completed. The upload
			// session TTL bounds abandoned bodies, while large chunks must not
			// inherit the general page/API read deadline.
			_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
			next.ServeHTTP(w, r)
		case http.MethodOptions, http.MethodHead, http.MethodDelete:
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Allow", "OPTIONS, HEAD, PATCH, DELETE")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})
}

func (s *Server) protected(privilege access.Privilege, handler http.HandlerFunc) http.HandlerFunc {
	return s.protect(privilege, handler).ServeHTTP
}

func (s *Server) protectedWithObjects(privilege access.Privilege, objectResolver httpauth.ObjectResolver, handler http.HandlerFunc) http.HandlerFunc {
	return s.protectWithObjects(privilege, objectResolver, handler).ServeHTTP
}

func (s *Server) protect(privilege access.Privilege, next http.Handler) http.Handler {
	return s.protectWithObjects(privilege, nil, next)
}

func (s *Server) protectWithObjects(privilege access.Privilege, objectResolver httpauth.ObjectResolver, next http.Handler) http.Handler {
	if s.auth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), principalContextKey{}, localDeveloperPrincipal())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return s.auth.MiddlewareWithObjectResolver(privilege, objectResolver, next)
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

func (s *Server) authLocalLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.LocalLogin(w, r)
}

func (s *Server) authLocalPassword(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.LocalPassword(w, r)
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		http.NotFound(w, r)
		return
	}
	s.auth.Logout(w, r)
}

func staticAssetCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := staticasset.Version()
		switch {
		case version != "dev" && r.URL.Query().Get("v") == version:
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case immutableStaticPath(r.URL.Path):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case fontStaticPath(r.URL.Path):
			w.Header().Set("Cache-Control", "public, max-age=86400")
		default:
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func immutableStaticPath(path string) bool {
	return strings.HasPrefix(path, "/static/chunks/")
}

func fontStaticPath(path string) bool {
	return strings.HasPrefix(path, "/static/files/") && strings.HasSuffix(path, ".woff2")
}

func favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="6" fill="#0969da"/><path d="M8 22h16v3H8zm1-5h4v4H9zm5-7h4v11h-4zm5 4h4v7h-4z" fill="#fff"/></svg>`))
}
