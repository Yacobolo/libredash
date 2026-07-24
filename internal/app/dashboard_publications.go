package app

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	queryauthz "github.com/Yacobolo/leapview/internal/analytics/query/authz"
	"github.com/Yacobolo/leapview/internal/dashboard"
	"github.com/Yacobolo/leapview/internal/dashboard/command"
	lddatastar "github.com/Yacobolo/leapview/internal/dashboard/datastar"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardhttp "github.com/Yacobolo/leapview/internal/dashboard/http"
	"github.com/Yacobolo/leapview/internal/dashboard/publication"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	dashboardsession "github.com/Yacobolo/leapview/internal/dashboard/session"
	reportui "github.com/Yacobolo/leapview/internal/dashboard/ui"
	"github.com/Yacobolo/leapview/internal/dataquery"
	"github.com/go-chi/chi/v5"
)

type resolvedPublicDashboard struct {
	publication publication.Publication
	metrics     dashboardhttp.Metrics
	report      dashboarddefinition.Definition
	modelID     string
}

func (s *Server) resolvePublicDashboard(ctx context.Context, publicID string) (resolvedPublicDashboard, error) {
	if s.publicationService == nil {
		return resolvedPublicDashboard{}, publication.ErrNotFound
	}
	row, err := s.publicationService.ResolvePublic(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return resolvedPublicDashboard{}, publication.ErrNotFound
	}
	metrics, ok := s.metricsForWorkspace(row.WorkspaceID)
	if !ok || metrics == nil {
		return resolvedPublicDashboard{}, publication.ErrNotFound
	}
	wrapped := dashboardhttp.Metrics(metrics)
	if s.store != nil {
		wrapped = dashboardCommandMetrics{QueryMetrics: metrics}
	}
	report, _, ok := wrapped.Report(row.Dashboard)
	if !ok {
		return resolvedPublicDashboard{}, publication.ErrNotFound
	}
	return resolvedPublicDashboard{publication: row, metrics: wrapped, report: report, modelID: wrapped.ModelIDForDashboard(row.Dashboard)}, nil
}

func (s *Server) publicDashboardDocument(presentation string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolved, err := s.resolvePublicDashboard(r.Context(), chi.URLParam(r, "publicId"))
		if err != nil {
			s.telemetry.publicDocumentObserved(presentation, "not_found")
			http.NotFound(w, r)
			return
		}
		pageID := strings.TrimSpace(chi.URLParam(r, "page"))
		if pageID == "" {
			pageID = resolved.publication.DefaultPage
		}
		pages := resolved.metrics.Pages(resolved.publication.Dashboard)
		activePage, ok := reportdef.ActivePage(pages, pageID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, model, ok := resolved.metrics.Report(resolved.publication.Dashboard)
		if !ok {
			http.NotFound(w, r)
			return
		}
		setPublicDashboardSecurityHeaders(w.Header(), presentation, resolved.publication.AllowedOrigins)
		s.telemetry.publicDocumentObserved(presentation, "success")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		initialFilters := resolved.report.FiltersFromURLForPage(activePage.ID, r.URL.Query())
		filterState, err := resolved.report.FilterStateFromURL(activePage.ID, r.URL.Query())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		initialFilters.CompiledState = &filterState
		if err := reportui.PublicPage(reportui.PublicPageOptions{
			PublicID: resolved.publication.PublicID, Presentation: presentation,
		}, resolved.metrics.Catalog(), resolved.report, model, pages, activePage, initialFilters).Render(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func (s *Server) publicDashboardUpdates(w http.ResponseWriter, r *http.Request) {
	resolved, err := s.resolvePublicDashboard(r.Context(), chi.URLParam(r, "publicId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	clientID := strings.TrimSpace(r.URL.Query().Get("clientId"))
	streamInstanceID := strings.TrimSpace(r.URL.Query().Get("streamInstance"))
	pageID := strings.TrimSpace(r.URL.Query().Get("page"))
	if pageID == "" {
		pageID = resolved.publication.DefaultPage
	}
	if clientID == "" || streamInstanceID == "" || !publicationPageExists(resolved.metrics.Pages(resolved.publication.Dashboard), pageID) {
		http.NotFound(w, r)
		return
	}
	presentation := strings.TrimSpace(r.URL.Query().Get("presentation"))
	if presentation != reportui.PresentationEmbed {
		presentation = reportui.PresentationPublic
	}
	streamID := lddatastar.StreamID(clientID, resolved.publication.Dashboard, pageID, streamInstanceID)
	version := publication.StreamVersion{PublicID: resolved.publication.PublicID, ServingStateID: resolved.publication.ServingStateID}
	initialFilters := resolved.report.NormalizeFiltersForPage(pageID, resolved.report.FiltersFromURLForPage(pageID, r.URL.Query()))
	filterState, err := resolved.report.FilterStateFromURL(pageID, r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	initialFilters.CompiledState = &filterState
	ctx, unregister, err := s.publicationStreams.Register(r.Context(), resolved.publication.ID, streamID, version, initialFilters)
	if err != nil {
		http.Error(w, "public dashboard stream is unavailable", http.StatusServiceUnavailable)
		return
	}
	defer unregister()
	streamFinished := s.telemetry.publicStreamStarted(presentation)
	defer streamFinished()
	query := r.URL.Query()
	query.Set("workspace", resolved.publication.WorkspaceID)
	query.Set("dashboard", resolved.publication.Dashboard)
	query.Set("model", resolved.modelID)
	query.Set("page", pageID)
	r.URL.RawQuery = query.Encode()
	ctx = s.publicDashboardExecutionContext(ctx, resolved)
	ctx = dashboardhttp.WithPublicPresentation(ctx, dashboardhttp.PublicPresentation{PublicID: resolved.publication.PublicID, Presentation: presentation})
	setPublicDashboardSecurityHeaders(w.Header(), presentation, resolved.publication.AllowedOrigins)
	s.publicDashboardHTTP(resolved).Updates(w, r.WithContext(ctx))
}

func (s *Server) publicDashboardCommand(commandName string, action func(dashboardhttp.Handler, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolved, err := s.resolvePublicDashboard(r.Context(), chi.URLParam(r, "publicId"))
		if err != nil {
			s.telemetry.publicCommandObserved(commandName, "not_found")
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		query.Set("workspace", resolved.publication.WorkspaceID)
		query.Set("dashboard", resolved.publication.Dashboard)
		query.Set("model", resolved.modelID)
		r.URL.RawQuery = query.Encode()
		ctx := s.publicDashboardExecutionContext(r.Context(), resolved)
		setPublicDashboardSecurityHeaders(w.Header(), reportui.PresentationPublic, resolved.publication.AllowedOrigins)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		action(s.publicDashboardHTTP(resolved), recorder, r.WithContext(ctx))
		outcome := "accepted"
		if recorder.status >= http.StatusBadRequest {
			outcome = "rejected"
		}
		s.telemetry.publicCommandObserved(commandName, outcome)
	}
}

func (s *Server) publicDashboardHTTP(resolved resolvedPublicDashboard) dashboardhttp.Handler {
	handler := s.dashboardHTTP()
	handler.Metrics = resolved.metrics
	handler.Broker = s.publicationBroker
	handler.MetricsForWorkspace = func(workspaceID string) (dashboardhttp.Metrics, bool) {
		return resolved.metrics, workspaceID == resolved.publication.WorkspaceID
	}
	handler.CSRFToken = nil
	handler.ChromeDecorators = nil
	handler.SessionKey = func(_ *http.Request, definition dashboarddefinition.Definition, clientID, streamInstanceID string) dashboardsession.Key {
		return dashboardsession.Key{
			WorkspaceOrPublication: resolved.publication.ID,
			PrincipalOrClient:      clientID,
			DashboardID:            definition.ID,
			ServingStateID:         resolved.publication.ServingStateID,
			StreamInstanceID:       streamInstanceID,
		}
	}
	handler.CommandGuard = func(r *http.Request, _ dashboardhttp.Metrics, request command.Request, signals dashboard.Signals) error {
		current, err := s.publicationRepo.GetByPublicID(r.Context(), resolved.publication.PublicID)
		if err != nil || current.Status() != publication.StatusActive || current.ID != resolved.publication.ID || current.ServingStateID != resolved.publication.ServingStateID {
			return publication.ErrNotFound
		}
		if request.DashboardID != resolved.publication.Dashboard || request.ModelID != resolved.modelID || !publicationPageExists(resolved.metrics.Pages(resolved.publication.Dashboard), request.PageID) {
			return fmt.Errorf("command target is outside publication")
		}
		if signals.Runtime.ClientID == "" || signals.Runtime.StreamInstanceID == "" {
			return fmt.Errorf("public command requires stream identity")
		}
		streamID := lddatastar.StreamID(signals.Runtime.ClientID, request.DashboardID, request.PageID, signals.Runtime.StreamInstanceID)
		version := publication.StreamVersion{PublicID: resolved.publication.PublicID, ServingStateID: resolved.publication.ServingStateID}
		if !s.publicationStreams.Active(resolved.publication.ID, streamID, version) {
			return fmt.Errorf("public command stream is not active")
		}
		return nil
	}
	handler.SharedCommandPrepare = func(r *http.Request, request command.Request, signals dashboard.Signals, prepare func(dashboard.Filters) (command.PreparedRefresh, error)) (command.PreparedRefresh, uint64, error) {
		streamID := lddatastar.StreamID(signals.Runtime.ClientID, request.DashboardID, request.PageID, signals.Runtime.StreamInstanceID)
		version := publication.StreamVersion{PublicID: resolved.publication.PublicID, ServingStateID: resolved.publication.ServingStateID}
		return s.publicationStreams.PrepareCommand(r.Context(), resolved.publication.ID, streamID, version, prepare)
	}
	return handler
}

func (s *Server) publicDashboardExecutionContext(ctx context.Context, resolved resolvedPublicDashboard) context.Context {
	principalID := access.DashboardPublicationSubjectID(resolved.publication.WorkspaceID, resolved.publication.Name)
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{
		WorkspaceID: resolved.publication.WorkspaceID, Surface: dataquery.SurfacePublicDashboard,
		PrincipalID: principalID, ObjectType: "dashboard_publication", ObjectID: resolved.publication.Name,
	})
	return queryauthz.WithDashboardPublicationCapability(ctx, queryauthz.DashboardPublicationCapability{
		WorkspaceID: resolved.publication.WorkspaceID, Publication: resolved.publication.Name,
		Dashboard: resolved.publication.Dashboard, ModelID: resolved.modelID,
		DependencyAssetIDs: append([]string(nil), resolved.publication.DependencyAssetIDs...),
	})
}

func publicationPageExists(pages []dashboard.Page, pageID string) bool {
	for _, page := range pages {
		if page.ID == pageID {
			return true
		}
	}
	return false
}

func setPublicDashboardSecurityHeaders(header http.Header, presentation string, origins []string) {
	frameAncestors := "'none'"
	if presentation == reportui.PresentationEmbed {
		header.Del("X-Frame-Options")
		if len(origins) > 0 {
			allowed := append([]string(nil), origins...)
			sort.Strings(allowed)
			frameAncestors = strings.Join(allowed, " ")
		}
	} else {
		header.Set("X-Frame-Options", "DENY")
	}
	header.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'", "base-uri 'none'", "object-src 'none'", "frame-ancestors " + frameAncestors,
		"form-action 'none'", "script-src 'self' 'unsafe-eval'", "style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:", "font-src 'self' data:", "connect-src 'self'", "worker-src 'self' blob:",
	}, "; "))
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Robots-Tag", "noindex")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", "no-store")
}
