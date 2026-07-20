package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/workspace"
)

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	if s == nil || s.store == nil {
		checks["platformStore"] = "missing"
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "not_ready", Checks: checks})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		checks["platformStore"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "not_ready", Checks: checks})
		return
	}
	checks["platformStore"] = "ok"
	if !s.runtimeReady(ctx, checks) {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "not_ready", Checks: checks})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ready", Checks: checks})
}

type activeWorkspaceLister interface {
	ListWithActiveMetadata(ctx context.Context, environment string) ([]workspace.Summary, error)
}

type workspaceRuntimeReadiness interface {
	RuntimeReady(ctx context.Context, workspaceID string) error
}

func (s *Server) runtimeReady(ctx context.Context, checks map[string]string) bool {
	if s == nil || s.metrics == nil {
		checks["runtime"] = "missing"
		return false
	}
	workspaces, err := s.activeRuntimeWorkspaces(ctx)
	if err != nil {
		checks["runtime"] = err.Error()
		return false
	}
	if len(workspaces) == 0 {
		checks["runtime"] = "no_active_deployments"
		return true
	}
	ready := true
	for _, workspaceID := range workspaces {
		checkName := "workspaceRuntime:" + workspaceID
		if err := s.workspaceRuntimeReady(ctx, workspaceID); err != nil {
			checks[checkName] = err.Error()
			ready = false
			continue
		}
		checks[checkName] = "ok"
	}
	return ready
}

func (s *Server) activeRuntimeWorkspaces(ctx context.Context) ([]string, error) {
	repo, err := s.workspaceRepository()
	if err != nil {
		return nil, err
	}
	lister, ok := repo.(activeWorkspaceLister)
	if !ok {
		if s.defaultWorkspaceID != "" {
			return []string{s.defaultWorkspaceID}, nil
		}
		catalog := s.metrics.Catalog()
		if catalog.Workspace.ID != "" && (len(catalog.Dashboards) > 0 || len(catalog.Models) > 0) {
			return []string{catalog.Workspace.ID}, nil
		}
		return nil, nil
	}
	summaries, err := lister.ListWithActiveMetadata(ctx, s.defaultEnvironment)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary.ID == "" || summary.ActiveServingStateID == "" {
			continue
		}
		out = append(out, string(summary.ID))
	}
	return out, nil
}

func (s *Server) workspaceRuntimeReady(ctx context.Context, workspaceID string) error {
	if readiness, ok := s.metrics.(workspaceRuntimeReadiness); ok {
		return readiness.RuntimeReady(ctx, workspaceID)
	}
	metrics, ok := s.metricsForWorkspace(workspaceID)
	if !ok || metrics == nil {
		return fmt.Errorf("runtime for workspace %q is not configured", workspaceID)
	}
	return metricsMetadataReady(metrics, workspaceID)
}

func metricsMetadataReady(metrics QueryMetrics, workspaceID string) error {
	catalog := metrics.Catalog()
	if workspaceID != "" && catalog.Workspace.ID != "" && catalog.Workspace.ID != workspaceID {
		return fmt.Errorf("catalog workspace = %q, want %q", catalog.Workspace.ID, workspaceID)
	}
	if len(catalog.Models) == 0 && len(catalog.Dashboards) == 0 {
		return fmt.Errorf("runtime catalog is empty")
	}
	defaultDashboardID := metrics.DefaultDashboardID()
	if len(catalog.Dashboards) == 0 {
		return nil
	}
	if defaultDashboardID == "" {
		return fmt.Errorf("default dashboard is not configured")
	}
	report, model, ok := metrics.Report(defaultDashboardID)
	return reportMetadataReady(metrics, defaultDashboardID, report, model, ok)
}

func reportMetadataReady(metrics interface {
	Pages(string) []dashboard.Page
}, dashboardID string, report reportdef.Dashboard, model any, ok bool) error {
	if !ok {
		return fmt.Errorf("default dashboard %q is not available", dashboardID)
	}
	if report.ID == "" {
		return fmt.Errorf("default dashboard %q has no report id", dashboardID)
	}
	if model == nil {
		return fmt.Errorf("default dashboard %q has no semantic model", dashboardID)
	}
	if len(metrics.Pages(dashboardID)) == 0 {
		return fmt.Errorf("default dashboard %q has no pages", dashboardID)
	}
	return nil
}
