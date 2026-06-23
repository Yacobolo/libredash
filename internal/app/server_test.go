package app

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func fieldRefs(fields ...string) []semantic.FieldRef {
	refs := make([]semantic.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = semantic.FieldRef{Field: field}
	}
	return refs
}

type fakeMetrics struct{}

type canceledTableMetrics struct {
	fakeMetrics
}

type recordingMetrics struct {
	fakeMetrics
	pageIDs []string
}

func (fakeMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "test-workspace", Title: "Test Workspace", Description: "Fixture workspace"},
		Models: []dashboard.CatalogModel{
			{ID: "test", Title: "Test Model", Description: "Fixture model"},
		},
		Dashboards: []dashboard.CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales Dashboard", Description: "Fixture report", SemanticModel: "test", Tags: []string{"sales"}, PageCount: 2},
		},
	}
}

func (fakeMetrics) DefaultDashboardID() string {
	return "executive-sales"
}

func (fakeMetrics) ModelIDForDashboard(dashboardID string) string {
	if dashboardID == "executive-sales" {
		return "test"
	}
	return ""
}

func (fakeMetrics) DataDir() string {
	return ".data/olist"
}

func (fakeMetrics) Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool) {
	if dashboardID != "executive-sales" {
		return semantic.Dashboard{}, nil, false
	}
	return semantic.Dashboard{
			ID:            "executive-sales",
			Title:         "Executive Sales Dashboard",
			SemanticModel: "test",
			Filters: map[string]semantic.FilterDefinition{
				"state":    {Type: "multi_select", Label: "State", Dimension: "orders.status", URLParam: "state", Operator: "in", Values: semantic.FilterValues{Source: "distinct", Limit: 50}},
				"category": {Type: "text", Label: "Category", Dimension: "orders.status", URLParam: "category", DefaultOperator: "contains", Operators: []string{"contains", "equals"}},
			},
			Visuals: map[string]semantic.Visual{
				"orders":       {Title: "Orders", Type: "donut", Query: semantic.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: semantic.Interaction{Field: "orders.status"}},
				"ops_pipeline": {Title: "Ops Pipeline", Type: "bar", Query: semantic.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: semantic.Interaction{Field: "orders.status"}},
			},
			Tables: map[string]semantic.TableVisual{
				"orders": {Title: "Orders", Query: semantic.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, DefaultSort: dashboard.TableSort{Key: "purchase_date", Direction: "desc"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
			},
			Pages: fakeMetrics{}.Pages(dashboardID),
		}, &semantic.Model{
			Name:  "test",
			Title: "Test Model",
			Tables: map[string]semantic.ModelTable{
				"orders": {
					Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
					Dimensions: map[string]semantic.MetricDimension{"order_id": {Expr: "order_id"}, "status": {Expr: "status"}},
				},
			},
			Measures: map[string]semantic.MetricMeasure{"order_count": {Table: "orders", Grain: "order_id", Label: "Orders", Expression: "COUNT(*)"}},
		}, true
}

func (fakeMetrics) DefaultFilters(_ string) dashboard.Filters {
	return dashboard.Filters{
		Controls: map[string]dashboard.FilterControl{
			"state":    {Type: "multi_select", Operator: "in", Values: []string{}},
			"category": {Type: "text", Operator: "contains"},
		},
		VisualSelections: []dashboard.VisualSelection{},
	}
}

func (fakeMetrics) NormalizeTableRequest(_ string, request dashboard.TableRequest) dashboard.TableRequest {
	return request.WithDefaults()
}

func (fakeMetrics) Pages(dashboardID string) []dashboard.Page {
	if dashboardID != "executive-sales" {
		return nil
	}
	return []dashboard.Page{
		{
			ID:     "overview",
			Title:  "Overview",
			Width:  1366,
			Height: 940,
			Visuals: []dashboard.PageVisual{
				{ID: "header", Kind: "header", X: 0, Y: 0, Width: 100, Height: 40, Title: "Test"},
				{ID: "state-filter", Kind: "filter_card", Filter: "state", X: 0, Y: 42, Width: 100, Height: 32},
				{ID: "orders-chart", Kind: "donut_chart", Visual: "orders", X: 0, Y: 48, Width: 100, Height: 100},
				{ID: "orders-table", Kind: "table", Table: "orders", X: 0, Y: 160, Width: 100, Height: 100},
			},
		},
		{
			ID:     "operations",
			Title:  "Operations",
			Width:  1366,
			Height: 940,
			Visuals: []dashboard.PageVisual{
				{ID: "category-filter", Kind: "filter_card", Filter: "category", X: 0, Y: 8, Width: 100, Height: 32},
				{ID: "ops-pipeline-chart", Kind: "bar_chart", Visual: "ops_pipeline", X: 0, Y: 48, Width: 100, Height: 100},
			},
		},
	}
}

func (fakeMetrics) QueryDashboard(_ context.Context, _ string, filters dashboard.Filters) (dashboard.Patch, error) {
	return fakeMetrics{}.QueryDashboardPage(context.Background(), "executive-sales", "", filters)
}

func (fakeMetrics) QueryDashboardPage(_ context.Context, _ string, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	chartID := "orders"
	chartTitle := "Orders"
	if pageID == "operations" {
		chartID = "ops_pipeline"
		chartTitle = "Ops Pipeline"
	}
	return dashboard.Patch{
		Filters: filters.WithDefaults(),
		FilterOptions: map[string][]dashboard.FilterOption{
			"state": {{Value: "SP", Label: "SP"}},
		},
		Status: dashboard.Status{
			Loading:       false,
			LastUpdated:   "12:00:00",
			DataDirectory: ".data/olist",
		},
		Visuals: map[string]dashboard.Visual{
			chartID: {Title: chartTitle, Unit: "orders", Data: []dashboard.Datum{{"label": "delivered", "value": 1}}},
		},
	}, nil
}

func (m *recordingMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	m.pageIDs = append(m.pageIDs, pageID)
	return m.fakeMetrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func TestPageRouteRendersRequestedYamlPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/operations", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-sub-sidebar`) {
		t.Fatalf("report page did not render sub sidebar:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("report page still rendered report sidebar:\n%s", body)
	}
	if !strings.Contains(body, `&#34;compact&#34;:true`) {
		t.Fatalf("report page did not compact the primary sidebar:\n%s", body)
	}
	if !strings.Contains(body, `/dashboards/executive-sales/pages/operations`) {
		t.Fatalf("report sidebar did not include operations page link:\n%s", body)
	}
	if strings.Contains(body, `class="page-tab`) {
		t.Fatalf("report header still rendered page tabs:\n%s", body)
	}
	decoded := html.UnescapeString(body)
	if strings.Contains(decoded, `"collapsible"`) || strings.Contains(decoded, `"numbered"`) {
		t.Fatalf("report sidebar should use default sub-sidebar behavior without chat overrides:\n%s", decoded)
	}
	if !strings.Contains(decoded, `2. Operations`) {
		t.Fatalf("report header did not include numbered active page title:\n%s", decoded)
	}
	if !strings.Contains(decoded, `"visuals":{"ops_pipeline"`) {
		t.Fatalf("operations page did not seed active page chart only:\n%s", decoded)
	}
	if strings.Contains(decoded, `"orders":{"version":3`) {
		t.Fatalf("operations page seeded off-page order chart:\n%s", decoded)
	}
	if !strings.Contains(decoded, `"tables":{}`) {
		t.Fatalf("operations page should seed no table placeholders:\n%s", decoded)
	}
}

func TestPageRouteSeedsPageScopedFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/overview?state=SP&state=RJ&category=ignored", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `/static/url-sync.js`) {
		t.Fatalf("page did not include url sync script:\n%s", body)
	}
	if !strings.Contains(body, `&#34;state&#34;:[&#34;RJ&#34;,&#34;SP&#34;]`) {
		t.Fatalf("page did not seed state url params:\n%s", body)
	}
	if !strings.Contains(body, `&#34;values&#34;:[&#34;RJ&#34;,&#34;SP&#34;]`) {
		t.Fatalf("page did not seed state filter values:\n%s", body)
	}
	if strings.Contains(body, `&#34;category&#34;`) {
		t.Fatalf("overview page seeded off-page category filter:\n%s", body)
	}
}

func TestPageRouteSeedsOperationsPageFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/operations?state=SP&category=ops", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `&#34;category&#34;:&#34;ops&#34;`) && !strings.Contains(body, `&#34;value&#34;:&#34;ops&#34;`) {
		t.Fatalf("operations page did not seed category URL filter:\n%s", body)
	}
	if strings.Contains(body, `&#34;state&#34;`) {
		t.Fatalf("operations page seeded off-page state filter:\n%s", body)
	}
}

func TestHTMLRoutesIncludeDatastarInspector(t *testing.T) {
	for _, path := range []string{
		"/login",
		"/",
		"/dashboards/executive-sales/pages/overview",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			assertDatastarInspector(t, rec.Body.String())
		})
	}
}

func assertDatastarInspector(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		`/static/datastar-inspector.js`,
		`<datastar-inspector`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing Datastar inspector marker %q:\n%s", want, body)
		}
	}
}

func TestHomeRouteRendersDashboardCatalog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `Dashboards`) {
		t.Fatalf("home missing dashboard catalog title:\n%s", body)
	}
	if !strings.Contains(body, `Executive Sales Dashboard`) {
		t.Fatalf("home missing dashboard card:\n%s", body)
	}
	if !strings.Contains(body, `href="/dashboards/executive-sales"`) {
		t.Fatalf("home missing dashboard link:\n%s", body)
	}
	for _, want := range []string{`Dashboards`, `/`, `Workspaces`, `/workspaces`, `Connections`, `/connections`, `Settings`, `/workspaces/test-workspace/permissions`} {
		if !strings.Contains(body, want) {
			t.Fatalf("home sidebar missing %q:\n%s", want, body)
		}
	}
	for _, notWant := range []string{`Metric Views`, `/metrics`, `Semantic Models`, `/models`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("home sidebar rendered removed navigation %q:\n%s", notWant, body)
		}
	}
	if strings.Contains(body, `<ld-sub-sidebar`) {
		t.Fatalf("dashboard catalog should not render sub sidebar:\n%s", body)
	}
}

func TestLoginRouteRendersAzureADLogin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-topology-background`) {
		t.Fatalf("login page did not render topology background component:\n%s", body)
	}
	if !strings.Contains(body, `Sign in with Azure Active Directory`) {
		t.Fatalf("login page did not render Azure AD button:\n%s", body)
	}
	if !strings.Contains(body, `data-init__delay`) {
		t.Fatalf("login page did not include lazy background init:\n%s", body)
	}
	if !strings.Contains(body, `libredash-login-background-init`) {
		t.Fatalf("login page did not dispatch login background init event:\n%s", body)
	}
	if !strings.Contains(body, `/static/topology-background.js`) {
		t.Fatalf("login page did not include lazy topology background asset:\n%s", body)
	}
}

func TestDashboardRouteRedirectsToFirstPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "/dashboards/executive-sales/pages/overview" {
		t.Fatalf("Location = %q, want first page", got)
	}
}

func TestUnknownPageRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/missing", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLegacyRoutesReturnNotFound(t *testing.T) {
	for _, path := range []string{
		"/pages/overview",
		"/model",
		"/models",
		"/models/test",
		"/metrics",
		"/metrics/orders",
		"/metrics/orders/measures",
		"/metrics/orders/dimensions",
		"/metrics/orders/usage",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func (fakeMetrics) QueryTable(_ context.Context, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return fakeMetrics{}.QueryTablePage(context.Background(), "executive-sales", "", dashboard.Filters{}, request)
}

func (fakeMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	request = request.WithDefaults()
	return dashboard.Table{
		Version: 2,
		Title:   "Orders",
		Columns: []dashboard.TableColumn{
			{Key: "order_id", Label: "Order"},
		},
		TotalRows:     1,
		AvailableRows: 1,
		IsCapped:      false,
		RowCap:        dashboard.TableInteractiveRowCap,
		ChunkSize:     dashboard.TableChunkSize,
		RowHeight:     dashboard.TableRowHeight,
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks: map[string]dashboard.TableBlock{
			"a": {
				Start:        request.Start,
				RequestSeq:   request.RequestSeq,
				ResetVersion: request.ResetVersion,
				Sort:         request.Sort,
				Rows:         []map[string]any{{"order_id": "o1"}},
			},
		},
	}, nil
}

func (canceledTableMetrics) QueryTable(_ context.Context, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return canceledTableMetrics{}.QueryTablePage(context.Background(), "executive-sales", "", dashboard.Filters{}, request)
}

func (canceledTableMetrics) QueryTablePage(_ context.Context, _ string, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	request = request.WithDefaults()
	return dashboard.EmptyTable(request, context.Canceled), nil
}

func (fakeMetrics) RefreshMaterializations(_ context.Context, _ string) error {
	return nil
}

func TestUpdatesStreamsDatastarPatchSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?dashboard=executive-sales&page=overview&datastar=%7B%22filters%22%3A%7B%22controls%22%3A%7B%22state%22%3A%7B%22type%22%3A%22multi_select%22%2C%22operator%22%3A%22in%22%2C%22values%22%3A%5B%22SP%22%5D%7D%2C%22category%22%3A%7B%22type%22%3A%22text%22%2C%22operator%22%3A%22contains%22%2C%22value%22%3A%22ignored%22%7D%7D%7D%7D", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: datastar-patch-signals") {
		t.Fatalf("body does not contain Datastar patch signal event:\n%s", body)
	}
	if !strings.Contains(body, `"values":["SP"]`) {
		t.Fatalf("body does not include decoded filter state:\n%s", body)
	}
	if strings.Contains(body, `"category"`) {
		t.Fatalf("body streamed off-page category filter:\n%s", body)
	}
}

func TestUpdatesStreamsPageScopedChartSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?dashboard=executive-sales&page=operations&datastar=%7B%22runtime%22%3A%7B%22clientId%22%3A%22test-client%22%2C%22dashboardId%22%3A%22executive-sales%22%2C%22pageId%22%3A%22operations%22%7D%7D", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"visuals":{"ops_pipeline"`) {
		t.Fatalf("updates did not stream active page chart:\n%s", body)
	}
	if strings.Contains(body, `"visuals":{"orders"`) {
		t.Fatalf("updates streamed off-page chart:\n%s", body)
	}
	if !strings.Contains(body, `"tables":{}`) {
		t.Fatalf("updates should stream empty tables for chart-only page:\n%s", body)
	}
	if strings.Contains(body, `"kpis"`) {
		t.Fatalf("updates streamed legacy KPI signal:\n%s", body)
	}
}

func TestRefreshMaterializationsCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/refresh-materializations", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestChartSelectCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"visualSelections":[]},"runtime":{"clientId":"test-client"},"visualCommand":{"visualId":"orders","field":"status","value":"delivered","label":"delivered"},"tableCommand":{"table":"orders","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/chart-select", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestPageCommandsQueryActivePage(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "chart select",
			path: "/commands/chart-select",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"visualSelections":[]},"visualCommand":{"visualId":"ops_pipeline","field":"status","value":"delivered","label":"delivered"},"tableCommand":{"block":"all","start":0,"count":50}}`,
		},
		{
			name: "clear selection",
			path: "/commands/clear-selection",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"visualSelections":[{"visualId":"ops_pipeline","field":"status","values":["delivered"]}]},"tableCommand":{"block":"all","start":0,"count":50}}`,
		},
		{
			name: "reset filters",
			path: "/commands/reset-filters",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"tableCommand":{"block":"all","start":200,"count":50}}`,
		},
		{
			name: "refresh materializations",
			path: "/commands/refresh-materializations",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations","modelId":"test"},"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"tableCommand":{"block":"all","start":0,"count":50}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &recordingMetrics{}
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			New(metrics).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if len(metrics.pageIDs) != 1 || metrics.pageIDs[0] != "operations" {
				t.Fatalf("queried page IDs = %#v, want [operations]", metrics.pageIDs)
			}
		})
	}
}

func TestClearSelectionCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"visualSelections":[{"visualId":"orders","field":"status","values":["delivered"]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/clear-selection", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestResetFiltersCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"visualSelections":[{"visualId":"orders","field":"status","values":["delivered"]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":200,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/reset-filters", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"a","start":400,"count":50,"requestSeq":42,"sort":{"key":"revenue","direction":"desc"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/table-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandDoesNotPublishCanceledQueries(t *testing.T) {
	server := New(canceledTableMetrics{})
	updates, unsubscribe := server.broker.subscribe("test-client:executive-sales:overview")
	defer unsubscribe()

	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"overview"},"tableCommand":{"table":"orders","block":"all","start":400,"count":50,"requestSeq":42}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/table-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	select {
	case patch := <-updates:
		t.Fatalf("received canceled table patch: %#v", patch)
	default:
	}
}
