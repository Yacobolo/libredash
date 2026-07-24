package app

import (
	"net/http"
	"sort"
	"strings"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	adminmodule "github.com/Yacobolo/leapview/internal/admin/module"
	agentmodule "github.com/Yacobolo/leapview/internal/agent/module"
	apihttpmiddleware "github.com/Yacobolo/leapview/internal/api/httpmiddleware"
	apiprotocol "github.com/Yacobolo/leapview/internal/api/protocol"
	apitransport "github.com/Yacobolo/leapview/internal/api/transport"
	dashboardmodule "github.com/Yacobolo/leapview/internal/dashboard/module"
	"github.com/Yacobolo/leapview/internal/staticasset"
	uitransport "github.com/Yacobolo/leapview/internal/ui/transport"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
	"github.com/go-chi/chi/v5"
)

func Routes(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy) http.Handler {
	mux := chi.NewRouter()
	csrf := func(next http.Handler) http.Handler {
		return csrfMiddleware(routes, runtime, platform, policy, next)
	}
	publicProtocol := func(next http.Handler) http.Handler {
		return publicProtocolMiddleware(routes, runtime, platform, policy, next)
	}
	if policy.requestLogging {
		mux.Use(apihttpmiddleware.RequestLogger(platform.logger))
	}
	mux.Use(platform.telemetry.Middleware)
	mux.Use(apihttpmiddleware.PanicRecovery(platform.logger))
	mux.Use(apihttpmiddleware.SecurityHeadersMiddleware(policy.securityHeaders))
	mux.Use(apihttpmiddleware.AllowedHosts(policy.allowedHosts))
	mux.Use(apihttpmiddleware.RequestBodyLimit(policy.requestBodyLimit))
	mux.Get("/favicon.ico", favicon)
	mux.Get("/healthz", platform.health.Healthz)
	mux.Get("/readyz", platform.health.Readyz)
	mux.Get("/api/openapi.json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAPIDescription(routes, runtime, platform, policy, w, r)
	}))
	mux.Get("/api/docs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { publicDocs(routes, runtime, platform, policy, w, r) }))
	mux.Group(func(r chi.Router) {
		r.Use(policy.rateLimits.PublicPage(func() { platform.telemetry.PublicRateLimitObserved("page") }))
		routes.dashboardModule.MountPublicDocuments(r)
	})
	mux.Group(func(r chi.Router) {
		r.Use(policy.rateLimits.PublicCommand(func() { platform.telemetry.PublicRateLimitObserved("command") }))
		routes.dashboardModule.MountPublicCommands(r)
	})
	routes.dashboardModule.MountPublicStream(mux.With(policy.rateLimits.PublicStream(func() { platform.telemetry.PublicRateLimitObserved("stream") })))
	if runtime.pageStreamTrace != nil {
		traceHandler := uitransport.TraceHandler{Store: runtime.pageStreamTrace}
		mux.Get("/__dev/pagestream/traces", traceHandler.Traces)
		mux.Get("/__dev/pagestream/signals", traceHandler.Signals)
	}
	mux.With(policy.rateLimits.Auth()).Handle("/metrics", platform.telemetry.MetricsHandler(policy.metricsBearerToken, accessmodule.BearerToken))
	mux.With(csrf).Group(routes.accessModule.MountLoginPage)
	mux.Group(func(r chi.Router) {
		r.Use(csrf)
		r.With(policy.rateLimits.Updates()).Get("/updates", runtime.pageStreams.ServeHTTP)
		r.Get("/", routes.accessModule.ProtectViewItem(routes.workspaceModule.Home))
		routes.workspaceModule.MountAuthenticated(r, workspacemodule.RouteGuard{
			Protect: routes.accessModule.Protect, ProtectWithObjects: routes.accessModule.ProtectWithObjects, AssetObjectRefs: routes.workspaceModule.AssetObjectRefs,
		})
		routes.agentModule.MountAuthenticated(r, agentmodule.RouteGuard{
			Protect: routes.accessModule.Protect, ProtectGlobal: routes.accessModule.ProtectGlobal,
		})
		r.Get("/chat", redirectLegacyChat)
		r.Get("/chat/updates", http.NotFound)
		r.Get("/chat/*", redirectLegacyChat)
		r.Post("/chat/turns", redirectLegacyChat)
		routes.adminModule.MountAuthenticated(r, adminmodule.RouteGuard{
			Protect: routes.accessModule.Protect, ProtectGlobal: routes.accessModule.ProtectGlobal,
			ProtectAnyWorkspace: routes.accessModule.ProtectAnyWorkspace,
		})
		routes.dashboardModule.MountAuthenticated(r, dashboardmodule.RouteGuard{
			Protect: routes.accessModule.Protect, ProtectWithObjects: routes.accessModule.ProtectWithObjects,
		})
		routes.accessModule.MountAuthenticatedBrowser(r)
	})
	mux.Group(func(r chi.Router) {
		r.Use(policy.rateLimits.Auth())
		r.Use(csrf)
		routes.accessModule.MountLocalLogin(r)
	})
	mux.Group(func(r chi.Router) {
		r.Use(policy.rateLimits.Auth())
		routes.accessModule.MountOAuthEndpoints(r)
	})
	routes.accessModule.MountOAuthMetadata(mux)
	if runtime.persistenceConfigured {
		if platform.auth != nil {
			mux.With(policy.rateLimits.API()).Handle("/mcp", routes.agentModule.MCPHandler())
		}
		if strings.TrimSpace(policy.scimBearerToken) != "" {
			if handler, err := routes.accessModule.SCIMHandler(policy.scimBearerToken); err == nil {
				scimHandler := policy.rateLimits.API()(http.StripPrefix("/scim", handler))
				mux.Handle("/scim/*", scimHandler)
			}
		}
		mux.Group(func(r chi.Router) {
			r.Use(policy.rateLimits.API())
			r.Use(publicProtocol)
			if policy.managedDataTus != nil {
				tus := routes.accessModule.ProtectIngestData(policy.managedDataTus)
				r.Handle("/upload-protocols/tus", tus)
				r.Handle("/upload-protocols/tus/*", tus)
			}
			registerAPIGenRoutes(routes, runtime, platform, policy, r)
		})
	}
	if routes.dashboardAssets != nil {
		mux.Handle("/map-assets/*", routes.dashboardAssets.Handler())
	}
	mux.Handle("/static/*", staticAssetCache(http.StripPrefix("/static/", http.FileServer(http.Dir("static")))))
	mux.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if isPublicAPIPath(r.URL.Path) {
			apiprotocol.PrepareRequest(w, r)
			apitransport.WriteProblem(w, r, http.StatusNotFound, "API_ROUTE_NOT_FOUND", "The requested API route does not exist", nil)
			return
		}
		http.NotFound(w, r)
	})
	registeredMethods := registeredRouteMethods(mux)
	mux.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		setAllowedMethods(w.Header(), mux, registeredMethods, r.URL.Path)
		if isPublicAPIPath(r.URL.Path) {
			if platform.apiProtocol.Authenticate(w, r) {
				apitransport.WriteProblem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "The requested method is not supported for this API route", nil)
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

func protectGlobalAgent(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, privilege accessmodule.Privilege, next http.Handler) http.Handler {
	return routes.accessModule.ProtectGlobal(privilege, next.ServeHTTP)
}

func protectAnyWorkspace(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, privilege accessmodule.Privilege, next http.Handler) http.Handler {
	return routes.accessModule.ProtectAnyWorkspace(privilege, next.ServeHTTP)
}

func protect(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, privilege accessmodule.Privilege, next http.Handler) http.Handler {
	return routes.accessModule.ProtectHandler(privilege, next)
}

func protectGlobal(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, privilege accessmodule.Privilege, next http.Handler) http.Handler {
	return routes.accessModule.ProtectGlobal(privilege, next.ServeHTTP)
}

func protectWithObjects(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, privilege accessmodule.Privilege, objectResolver accessmodule.ObjectResolver, next http.Handler) http.Handler {
	return routes.accessModule.ProtectHandlerWithObjects(privilege, objectResolver, next)
}

func csrfMiddleware(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, next http.Handler) http.Handler {
	return routes.accessModule.CSRFMiddleware(next)
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
