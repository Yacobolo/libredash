package http

import (
	"context"
	"html"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/go-chi/chi/v5"
)

type fakeMetrics struct{}

func (fakeMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{Workspace: dashboard.CatalogWorkspace{ID: "workspace", Title: "Workspace"}}
}
func (fakeMetrics) DataDir() string { return ".data" }
func (fakeMetrics) DefaultDashboardID() string {
	return "dash"
}
func (fakeMetrics) DefaultFilters(string) dashboard.Filters {
	return dashboard.Filters{}.WithDefaults()
}
func (fakeMetrics) ModelIDForDashboard(string) string {
	return "model"
}
func (fakeMetrics) NormalizeTableRequest(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	return request.WithDefaults()
}
func (fakeMetrics) Pages(dashboardID string) []dashboard.Page {
	if dashboardID != "dash" {
		return nil
	}
	return []dashboard.Page{{ID: "overview", Title: "Overview"}, {ID: "ops", Title: "Ops"}}
}
func (fakeMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if dashboardID != "dash" {
		return reportdef.Dashboard{}, nil, false
	}
	return reportdef.Dashboard{
		ID:     "dash",
		Title:  "Dashboard",
		Tables: map[string]reportdef.TableVisual{},
		Pages:  fakeMetrics{}.Pages(dashboardID),
	}, &semanticmodel.Model{Name: "model", Title: "Model"}, true
}
func (fakeMetrics) QueryDashboardPage(_ context.Context, _ string, _ string, filters dashboard.Filters) (dashboard.Patch, error) {
	return dashboard.Patch{Filters: filters.WithDefaults(), Status: dashboard.Status{DataDirectory: ".data"}}, nil
}
func (fakeMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return dashboard.Table{Title: request.Table}, nil
}
func (fakeMetrics) RefreshMaterializations(context.Context, string) error {
	return nil
}

func TestDashboardRedirectsToFirstPage(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodGet, "/dashboards/dash", nil)

	testRouter(Handler{Metrics: fakeMetrics{}}).ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/dashboards/dash/pages/overview" {
		t.Fatalf("Location = %q", got)
	}
}

func TestPageNotFound(t *testing.T) {
	for _, path := range []string{"/dashboards/missing/pages/overview", "/dashboards/dash/pages/missing"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(nethttp.MethodGet, path, nil)

			testRouter(Handler{Metrics: fakeMetrics{}}).ServeHTTP(rec, req)

			if rec.Code != nethttp.StatusNotFound {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPageSetsClientCookieAndRendersReport(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(nethttp.MethodGet, "/dashboards/dash/pages/overview", nil)

	testRouter(Handler{Metrics: fakeMetrics{}}).ServeHTTP(rec, req)

	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "ld_client_id" || cookies[0].Value == "" {
		t.Fatalf("cookies = %#v", cookies)
	}
	body := html.UnescapeString(rec.Body.String())
	if !strings.Contains(body, `<ld-dashboard-page`) || !strings.Contains(body, `/updates?dashboard=dash&page=overview`) {
		t.Fatalf("page did not render report shell:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-canvas`) {
		t.Fatalf("page rendered dashboard internals in Go shell:\n%s", body)
	}
}

func testRouter(handler Handler) nethttp.Handler {
	r := chi.NewRouter()
	r.Get("/dashboards/{dashboard}", handler.Dashboard)
	r.Get("/dashboards/{dashboard}/pages/{page}", handler.Page)
	return r
}
