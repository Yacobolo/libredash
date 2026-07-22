package app

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/access/httpauth"
	"github.com/Yacobolo/leapview/internal/access/scimprov"
	dashboardhttp "github.com/Yacobolo/leapview/internal/dashboard/http"
	reportui "github.com/Yacobolo/leapview/internal/dashboard/ui"
	"github.com/Yacobolo/leapview/internal/staticasset"
	workspacehttp "github.com/Yacobolo/leapview/internal/workspace/http"
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
	mux.Get("/api/openapi.json", s.openAPIDescription)
	mux.Get("/api/docs", s.publicDocs)
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.publicPageMiddleware(s.telemetry))
		r.Get("/public/dashboards/{publicId}", s.publicDashboardDocument(reportui.PresentationPublic))
		r.Get("/public/dashboards/{publicId}/pages/{page}", s.publicDashboardDocument(reportui.PresentationPublic))
		r.Get("/embed/dashboards/{publicId}", s.publicDashboardDocument(reportui.PresentationEmbed))
		r.Get("/embed/dashboards/{publicId}/pages/{page}", s.publicDashboardDocument(reportui.PresentationEmbed))
	})
	mux.Group(func(r chi.Router) {
		r.Use(s.rateLimits.publicCommandMiddleware(s.telemetry))
		r.Post("/public/dashboards/{publicId}/commands/reload", s.publicDashboardCommand("reload", func(h dashboardhttp.Handler, w http.ResponseWriter, r *http.Request) { h.Reload(w, r) }))
		r.Post("/public/dashboards/{publicId}/commands/reset-filters", s.publicDashboardCommand("reset_filters", func(h dashboardhttp.Handler, w http.ResponseWriter, r *http.Request) { h.ResetFilters(w, r) }))
		r.Post("/public/dashboards/{publicId}/commands/select", s.publicDashboardCommand("select", func(h dashboardhttp.Handler, w http.ResponseWriter, r *http.Request) { h.Select(w, r) }))
		r.Post("/public/dashboards/{publicId}/commands/clear-selection", s.publicDashboardCommand("clear_selection", func(h dashboardhttp.Handler, w http.ResponseWriter, r *http.Request) { h.ClearSelection(w, r) }))
		r.Post("/public/dashboards/{publicId}/commands/visual-window", s.publicDashboardCommand("visual_window", func(h dashboardhttp.Handler, w http.ResponseWriter, r *http.Request) { h.VisualWindow(w, r) }))
	})
	mux.With(s.rateLimits.publicStreamMiddleware(s.telemetry)).Get("/public/dashboards/{publicId}/updates", s.publicDashboardUpdates)
	if s.pageStreamTrace != nil {
		mux.Get("/__dev/pagestream/traces", s.pageStreamTraces)
		mux.Get("/__dev/pagestream/signals", s.pageStreamSignals)
	}
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
		r.Get("/workspaces/{workspace}/assets/{asset}", s.protectedWithObjects(access.PrivilegeViewItem, s.workspaceAssetObjectRefs, workspaceHTTP.WorkspaceAsset))
		r.Get("/workspaces/{workspace}/assets/{asset}/{section}", s.protectedWithObjects(access.PrivilegeViewItem, s.workspaceAssetObjectRefs, workspaceHTTP.WorkspaceAssetSection))
		r.Post("/workspaces/{workspace}/assets/{asset}/refresh", s.protectedWithObjects(access.PrivilegeRefreshData, s.workspaceAssetObjectRefs, workspaceHTTP.RefreshAsset))
		r.Get("/workspaces/{workspace}/data", s.protected(access.PrivilegeViewItem, workspaceHTTP.WorkspaceDataExplorerRedirect))
		agentHTTP := s.agentHTTPHandler()
		r.Get("/chats", s.globalAgentProtected(access.PrivilegeViewAgent, agentHTTP.Chat))
		r.Get("/chats/new", s.globalAgentProtected(access.PrivilegeViewAgent, agentHTTP.ChatNew))
		r.Get("/chats/references/search", s.globalAgentProtected(access.PrivilegeViewItem, agentHTTP.ChatReferenceSearch))
		r.Get("/chats/restore", s.globalAgentProtected(access.PrivilegeViewAgent, agentHTTP.ChatRestore))
		r.Get("/chats/{conversation}", s.globalAgentProtected(access.PrivilegeViewAgent, agentHTTP.ChatConversation))
		r.Post("/chats/turns", s.globalAgentProtected(access.PrivilegeUseAgent, agentHTTP.ChatTurn))
		r.Get("/chat", redirectLegacyChat)
		r.Get("/chat/updates", http.NotFound)
		r.Get("/chat/*", redirectLegacyChat)
		r.Post("/chat/turns", redirectLegacyChat)
		adminHTTP := s.adminHTTPHandler()
		r.Get("/admin", s.protected(access.PrivilegeManageGrants, adminHTTP.General))
		r.Get("/admin/principals", s.protected(access.PrivilegeManageGrants, adminHTTP.Principals))
		r.Get("/admin/principals/{principal}", s.protected(access.PrivilegeManageGrants, adminHTTP.PrincipalDetail))
		r.Get("/admin/groups", s.protected(access.PrivilegeManageGrants, adminHTTP.Groups))
		r.Get("/admin/groups/{group}", s.protected(access.PrivilegeManageGrants, adminHTTP.GroupDetail))
		r.Get("/admin/agent", s.protected(access.PrivilegeManageGrants, adminHTTP.Agent))
		r.Patch("/admin/agent/config", s.protected(access.PrivilegeManageGrants, agentHTTP.UpdateAdminConfig))
		r.Get("/admin/storage", s.protected(access.PrivilegeManageGrants, adminHTTP.Storage))
		r.Post("/admin/storage/select-table", s.protected(access.PrivilegeManageGrants, adminHTTP.StorageTableSelect))
		r.Get("/admin/queries", s.protected(access.PrivilegeViewAudit, adminHTTP.Queries))
		r.Post("/admin/queries/command", s.protected(access.PrivilegeViewAudit, adminHTTP.QueryCommand))
		r.Get("/admin/publications", s.protectedAnyWorkspace(access.PrivilegeManagePublications, adminHTTP.Publications))
		r.Post("/admin/publications/command", s.protectedAnyWorkspace(access.PrivilegeManagePublications, adminHTTP.PublicationCommand))
		r.Post("/workspaces/{workspace}/access/upsert", s.protected(access.PrivilegeManageGrants, workspaceHTTP.AccessUpsert))
		r.Get("/workspaces/{workspace}/access/search", s.protected(access.PrivilegeManageGrants, workspaceHTTP.AccessSearch))
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
		r.Post("/workspaces/{workspace}/commands/visual-window", s.protected(access.PrivilegeViewItem, dashboardHTTP.VisualWindow))
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
		r.Post("/oauth/token", s.oauthToken)
		r.Post("/oauth/register", s.mcpOAuthRegister)
		r.Post("/oauth/revoke", s.mcpOAuthRevoke)
	})
	mux.Get("/.well-known/oauth-protected-resource", s.mcpProtectedResourceMetadata)
	mux.Get("/.well-known/oauth-protected-resource/mcp", s.mcpProtectedResourceMetadata)
	mux.Get("/.well-known/oauth-authorization-server", s.mcpAuthorizationServerMetadata)
	if s.auth != nil {
		authorize := s.auth.Middleware("", http.HandlerFunc(s.mcpOAuthAuthorize))
		mux.Method(http.MethodGet, "/oauth/authorize", s.csrf(authorize))
		mux.Method(http.MethodPost, "/oauth/authorize", s.csrf(authorize))
	}
	if s.store != nil {
		if s.auth != nil {
			mux.With(s.rateLimits.apiMiddleware()).Handle("/mcp", s.mcpHandler())
		}
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
			r.Use(s.publicProtocolMiddleware)
			if s.managedDataTus != nil {
				tus := s.protect(access.PrivilegeIngestData, managedDataTusHandler(s.managedDataTus))
				r.Handle("/upload-protocols/tus", tus)
				r.Handle("/upload-protocols/tus/*", tus)
			}
			s.registerAPIGenRoutes(r)
		})
	}
	mux.Handle("/static/*", staticAssetCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))
	mux.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isPublicAPIPath(r.URL.Path) {
			preparePublicAPIRequest(w, r)
			writeAPIProblem(w, r, http.StatusNotFound, "API_ROUTE_NOT_FOUND", "The requested API route does not exist", nil)
			return
		}
		http.NotFound(w, r)
	})
	registeredMethods := registeredRouteMethods(mux)
	mux.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		setAllowedMethods(w.Header(), mux, registeredMethods, r.URL.Path)
		if isPublicAPIPath(r.URL.Path) {
			if s.authenticatePublicAPIRequest(w, r) {
				writeAPIProblem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The requested method is not supported for this API route", nil)
			}
			return
		}
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})

	return mux
}

func registeredRouteMethods(routes chi.Routes) []string {
	registered := make(map[string]struct{})
	_ = chi.Walk(routes, func(method, _ string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != "*" {
			registered[method] = struct{}{}
		}
		return nil
	})
	methods := make([]string, 0, len(registered))
	for method := range registered {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return methods
}

func setAllowedMethods(header http.Header, routes chi.Routes, methods []string, path string) {
	for _, method := range methods {
		if routes.Match(chi.NewRouteContext(), method, path) {
			header.Add("Allow", method)
		}
	}
}

func isPublicAPIPath(path string) bool {
	return path == "/api/v1" || strings.HasPrefix(path, "/api/v1/") || path == "/upload-protocols" || strings.HasPrefix(path, "/upload-protocols/")
}

func redirectLegacyChat(w http.ResponseWriter, r *http.Request) {
	target := "/chats" + strings.TrimPrefix(r.URL.Path, "/chat")
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
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

func (s *Server) protectedAnyWorkspace(privilege access.Privilege, handler http.HandlerFunc) http.HandlerFunc {
	return s.protectAnyWorkspace(privilege, handler).ServeHTTP
}

func (s *Server) globalAgentProtected(privilege access.Privilege, handler http.HandlerFunc) http.HandlerFunc {
	return s.protectGlobalAgent(privilege, handler).ServeHTTP
}

func (s *Server) protectGlobalAgent(privilege access.Privilege, next http.Handler) http.Handler {
	if s.auth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), principalContextKey{}, localDeveloperPrincipal())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return s.auth.Middleware("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := s.auth.Principal(r)
		if !ok {
			writeAuthError(w, r, errUnauthorized, http.StatusUnauthorized)
			return
		}
		if principal.DevBypass {
			next.ServeHTTP(w, r)
			return
		}
		var credential *access.APICredential
		if resolved, ok := s.auth.APICredential(r); ok {
			credential = &resolved
		}
		allowed, err := s.authorizeGlobalAgentPrivilege(r.Context(), principal.ID, credential, privilege)
		if err != nil {
			writeAuthError(w, r, err, http.StatusInternalServerError)
			return
		}
		if !allowed {
			writeAuthError(w, r, errForbidden, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) authorizeGlobalAgentPrivilege(ctx context.Context, principalID string, credential *access.APICredential, privilege access.Privilege) (bool, error) {
	return s.authorizeAnyWorkspacePrivilege(ctx, principalID, credential, privilege)
}

func (s *Server) authorizeAnyWorkspacePrivilege(ctx context.Context, principalID string, credential *access.APICredential, privilege access.Privilege) (bool, error) {
	workspaceRepo, err := s.workspaceRepository()
	if err != nil {
		return false, err
	}
	accessRepo, err := s.accessRepository()
	if err != nil {
		return false, err
	}
	workspaces, err := workspaceRepo.List(ctx)
	if err != nil {
		return false, err
	}
	objects := make([]access.ObjectRef, 0, len(workspaces))
	for _, item := range workspaces {
		workspaceID := string(item.ID)
		if credential != nil && !apiTokenAllows(credential.Token, workspaceID, privilege) {
			continue
		}
		objects = append(objects, access.WorkspaceObject(workspaceID))
	}
	decision, err := accessRepo.AuthorizeAny(ctx, principalID, privilege, objects)
	return decision.Allowed, err
}

func (s *Server) protectAnyWorkspace(privilege access.Privilege, next http.Handler) http.Handler {
	if s.auth == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), principalContextKey{}, localDeveloperPrincipal())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return s.auth.Middleware("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := s.auth.Principal(r)
		if !ok {
			writeAuthError(w, r, errUnauthorized, http.StatusUnauthorized)
			return
		}
		if principal.DevBypass {
			next.ServeHTTP(w, r)
			return
		}
		var credential *access.APICredential
		if resolved, ok := s.auth.APICredential(r); ok {
			credential = &resolved
		}
		allowed, err := s.authorizeAnyWorkspacePrivilege(r.Context(), principal.ID, credential, privilege)
		if err != nil {
			writeAuthError(w, r, err, http.StatusInternalServerError)
			return
		}
		if !allowed {
			writeAuthError(w, r, errForbidden, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
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
