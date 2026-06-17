package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
)

type fakeMetrics struct{}

type canceledTableMetrics struct {
	fakeMetrics
}

func (fakeMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "test-workspace", Title: "Test Workspace", Description: "Fixture workspace"},
		Models: []dashboard.CatalogModel{
			{ID: "test", Title: "Test Model", Description: "Fixture model"},
		},
		MetricViews: []dashboard.CatalogMetricView{
			{ID: "orders", Title: "Orders Metrics", Description: "Fixture metrics view", SemanticModel: "test", ModelTitle: "Test Model"},
		},
		Dashboards: []dashboard.CatalogDashboard{
			{ID: "executive-sales", Title: "Executive Sales Dashboard", Description: "Fixture report", MetricViews: []string{"orders"}, MetricViewTitles: []string{"Orders Metrics"}, Tags: []string{"sales"}, PageCount: 2},
		},
	}
}

func (fakeMetrics) MetricViews() []dashboard.MetricViewSummary {
	return []dashboard.MetricViewSummary{
		{
			ID:             "orders",
			Title:          "Orders Metrics",
			Description:    "Fixture metrics view",
			SemanticModel:  "test",
			ModelTitle:     "Test Model",
			Dataset:        "orders",
			Timeseries:     "purchase_timestamp",
			DimensionCount: 2,
			MeasureCount:   2,
			DashboardCount: 1,
		},
	}
}

func (fakeMetrics) MetricView(id string) (dashboard.MetricViewDetail, bool) {
	if id != "orders" {
		return dashboard.MetricViewDetail{}, false
	}
	return dashboard.MetricViewDetail{
		MetricViewSummary: dashboard.MetricViewSummary{
			ID:             "orders",
			Title:          "Orders Metrics",
			Description:    "Fixture metrics view",
			SemanticModel:  "test",
			ModelTitle:     "Test Model",
			Dataset:        "orders",
			Timeseries:     "purchase_timestamp",
			DimensionCount: 2,
			MeasureCount:   2,
			DashboardCount: 1,
		},
		Dimensions: []dashboard.MetricViewDimension{
			{Name: "category", Label: "Category", Expr: "e.category"},
			{Name: "delivery_bucket", Label: "Delivery speed", Expr: "e.delivery_bucket", Where: "e.delivery_bucket IS NOT NULL", OrderExpr: "MIN(e.delivery_days)"},
		},
		Measures: []dashboard.MetricViewMeasure{
			{Name: "order_count", Label: "Orders", Expression: "COUNT(DISTINCT e.order_id)", Unit: "orders", Format: "integer"},
			{Name: "revenue", Label: "Revenue", Expression: "SUM(e.revenue)", Unit: "R$", Format: "currency", Description: "Total paid revenue"},
		},
		Dashboards: []dashboard.MetricViewDashboard{
			{ID: "executive-sales", Title: "Executive Sales Dashboard", Description: "Fixture report", Tags: []string{"sales"}, PageCount: 2},
		},
	}, true
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
			ID:          "executive-sales",
			Title:       "Executive Sales Dashboard",
			MetricViews: []string{"orders"},
			Filters: map[string]semantic.FilterDefinition{
				"state": {Type: "multi_select", Label: "State", MetricView: "orders", Dimension: "status", URLParam: "state", Operator: "in", Values: semantic.FilterValues{Source: "distinct", Limit: 50}},
			},
			Visuals: map[string]semantic.Visual{
				"orders": {Title: "Orders", Type: "donut", MetricView: "orders", Query: semantic.VisualQuery{Dimensions: []string{"status"}, Measures: []string{"order_count"}}, Interaction: semantic.Interaction{Field: "status"}},
			},
			Tables: map[string]semantic.TableVisual{
				"orders": {Title: "Orders", MetricView: "orders", DefaultSort: dashboard.TableSort{Key: "purchase_date", Direction: "desc"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
			},
			Pages: fakeMetrics{}.Pages(dashboardID),
		}, &semantic.Model{
			Name:  "test",
			Title: "Test Model",
			Datasets: map[string]semantic.Dataset{
				"orders": {
					Source: "orders_enriched",
				},
			},
		}, true
}

func (fakeMetrics) DefaultFilters(_ string) dashboard.Filters {
	return dashboard.Filters{
		Controls: map[string]dashboard.FilterControl{
			"state": {Type: "multi_select", Operator: "in", Values: []string{}},
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
			},
		},
		{
			ID:     "operations",
			Title:  "Operations",
			Width:  1366,
			Height: 940,
		},
	}
}

func (fakeMetrics) ModelGraph(modelID string) (dashboard.ModelGraph, bool) {
	if modelID != "test" {
		return dashboard.ModelGraph{}, false
	}
	return dashboard.ModelGraph{
		Name:  "test",
		Title: "Test Model",
		Stats: dashboard.ModelStats{Sources: 1, CacheTables: 1, Relationships: 1},
		Nodes: []dashboard.ModelNode{
			{ID: "source:orders", Label: "orders", Kind: "source"},
			{ID: "cache:orders_enriched", Label: "orders_enriched", Kind: "cache"},
			{ID: "dataset:orders", Label: "orders", Kind: "dataset"},
			{ID: "metrics_view:orders", Label: "Orders Metrics", Kind: "metrics_view"},
		},
		Edges: []dashboard.ModelEdge{
			{ID: "orders_cache", Source: "source:orders", Target: "cache:orders_enriched", Kind: "materialization"},
			{ID: "orders_dataset", Source: "cache:orders_enriched", Target: "dataset:orders", Kind: "dataset"},
			{ID: "orders_metrics", Source: "dataset:orders", Target: "metrics_view:orders", Kind: "metrics"},
		},
	}, true
}

func (fakeMetrics) QueryDashboard(_ context.Context, _ string, filters dashboard.Filters) (dashboard.Patch, error) {
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
		KPIs: []dashboard.KPI{{Label: "Orders", Value: "1", Note: "test", Tone: "ink"}},
		Charts: map[string]dashboard.Chart{
			"orders": {Title: "Orders", Unit: "orders", Data: []dashboard.Datum{{"label": "delivered", "value": 1}}},
		},
	}, nil
}

func TestPageRouteRendersRequestedYamlPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/operations", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("report page did not render report sidebar:\n%s", body)
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
}

func TestPageRouteSeedsFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboards/executive-sales/pages/overview?state=SP&state=RJ", nil)
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
}

func TestHTMLRoutesIncludeDatastarInspector(t *testing.T) {
	for _, path := range []string{
		"/login",
		"/",
		"/dashboards/executive-sales/pages/overview",
		"/models",
		"/models/test",
		"/metrics",
		"/metrics/orders/measures",
		"/metrics/orders/dimensions",
		"/metrics/orders/usage",
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

func TestCatalogRouteRendersDashboardCatalog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `Executive Sales Dashboard`) {
		t.Fatalf("catalog missing dashboard title:\n%s", body)
	}
	if !strings.Contains(body, `href="/dashboards/executive-sales"`) {
		t.Fatalf("catalog missing dashboard link:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("catalog should not render report sidebar:\n%s", body)
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
	if !strings.Contains(body, `/static/login.js`) {
		t.Fatalf("login page did not include login asset:\n%s", body)
	}
}

func TestModelsRouteRendersSemanticModelCatalog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `Semantic Models`) {
		t.Fatalf("models catalog missing title:\n%s", body)
	}
	if !strings.Contains(body, `Test Model`) {
		t.Fatalf("models catalog missing model card:\n%s", body)
	}
	if !strings.Contains(body, `href="/models/test"`) {
		t.Fatalf("models catalog missing model link:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("models catalog should not render report sidebar:\n%s", body)
	}
}

func TestMetricsRouteRendersMetricViewCatalog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `Metric Views`) {
		t.Fatalf("metric view catalog missing title:\n%s", body)
	}
	if !strings.Contains(body, `Orders Metrics`) {
		t.Fatalf("metric view catalog missing metric view card:\n%s", body)
	}
	if !strings.Contains(body, `href="/metrics/orders/measures"`) {
		t.Fatalf("metric view catalog missing detail link:\n%s", body)
	}
	if !strings.Contains(body, `Metric Views`) || !strings.Contains(body, `/metrics`) {
		t.Fatalf("sidebar missing metric views navigation:\n%s", body)
	}
}

func TestMetricViewRouteRedirectsToMeasures(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/orders", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got, want := rec.Header().Get("Location"), "/metrics/orders/measures"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestMetricViewMeasuresRouteRendersMeasuresTab(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/orders/measures", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`Orders Metrics`,
		`href="/models/test"`,
		`<code>orders</code>`,
		`<code>purchase_timestamp</code>`,
		`href="/metrics/orders/measures"`,
		`href="/metrics/orders/dimensions"`,
		`href="/metrics/orders/usage"`,
		`aria-current="page"`,
		`/static/detail-rail.js`,
		`<ld-detail-rail class="metric-workspace">`,
		`data-signals=`,
		`metricGrid`,
		`<ld-data-grid data-attr:grid="$metricGrid"></ld-data-grid>`,
		`Revenue`,
		`SUM(e.revenue)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metric view measures tab missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `metric-contract-rail`) || strings.Contains(body, `metric-rail-section`) {
		t.Fatalf("metric view detail should not render the old right rail:\n%s", body)
	}
	for _, notWant := range []string{`>Measures</h2>`, `>Dimensions</h2>`, `>Used by</h2>`, `metric-section-header`, `metricUsageGraph`, `<ld-metric-usage-graph`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("metric view measures tab should not render %q:\n%s", notWant, body)
		}
	}
}

func TestMetricViewDimensionsRouteRendersDimensionsTab(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/orders/dimensions", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`aria-current="page"`,
		`Category`,
		`e.category`,
		`Delivery speed`,
		`e.delivery_bucket IS NOT NULL`,
		`metricGrid`,
		`<ld-data-grid data-attr:grid="$metricGrid"></ld-data-grid>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metric view dimensions tab missing %q:\n%s", want, body)
		}
	}
	for _, notWant := range []string{`>Measures</h2>`, `>Dimensions</h2>`, `>Used by</h2>`, `metric-section-header`, `SUM(e.revenue)`, `<ld-metric-usage-graph`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("metric view dimensions tab should not render %q:\n%s", notWant, body)
		}
	}
}

func TestMetricViewUsageRouteRendersUsageTab(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/orders/usage", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`aria-current="page"`,
		`data-signals=`,
		`metricUsageGraph`,
		`<ld-metric-usage-graph data-attr:graph="$metricUsageGraph"></ld-metric-usage-graph>`,
		`metricGrid`,
		`<ld-data-grid data-attr:grid="$metricGrid"></ld-data-grid>`,
		`Dashboard`,
		`Tags`,
		`/dashboards/executive-sales`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metric view usage tab missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `data-graph=`) {
		t.Fatalf("metric view detail should use signals instead of a serialized data-graph attribute:\n%s", body)
	}
	for _, notWant := range []string{`>Measures</h2>`, `>Dimensions</h2>`, `>Used by</h2>`, `metric-section-header`, `SUM(e.revenue)`, `e.category`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("metric view usage tab should not render %q:\n%s", notWant, body)
		}
	}
}

func TestUnknownMetricViewRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/missing", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUnknownMetricViewSectionRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics/orders/missing", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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
	for _, path := range []string{"/pages/overview", "/model"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestModelRouteRendersSemanticModelGraph(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/models/test", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-model-graph`) {
		t.Fatalf("body does not render model graph component:\n%s", body)
	}
	if !strings.Contains(body, `Test Model`) {
		t.Fatalf("body does not include model title:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("model page should not render report sidebar:\n%s", body)
	}
}

func (fakeMetrics) QueryTable(_ context.Context, _ string, _ dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
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
	request = request.WithDefaults()
	return dashboard.EmptyTable(request, context.Canceled), nil
}

func (fakeMetrics) RefreshCache(_ context.Context, _ string) error {
	return nil
}

func TestUpdatesStreamsDatastarPatchSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?dashboard=executive-sales&page=overview&datastar=%7B%22filters%22%3A%7B%22controls%22%3A%7B%22state%22%3A%7B%22type%22%3A%22multi_select%22%2C%22operator%22%3A%22in%22%2C%22values%22%3A%5B%22SP%22%5D%7D%7D%7D%7D", nil)
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
}

func TestRefreshCacheCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":0,"count":200}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/refresh-cache", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestChartSelectCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"visualSelections":[]},"runtime":{"clientId":"test-client"},"chartCommand":{"visualId":"orders","field":"status","value":"delivered","label":"delivered"},"tableCommand":{"table":"orders","block":"all","start":0,"count":200}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/chart-select", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestClearSelectionCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"visualSelections":[{"visualId":"orders","field":"status","values":["delivered"]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":0,"count":200}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/clear-selection", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestResetFiltersCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"visualSelections":[{"visualId":"orders","field":"status","values":["delivered"]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":200,"count":200}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands/reset-filters", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"a","start":400,"count":200,"requestSeq":42,"sort":{"key":"revenue","direction":"desc"}}}`)
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

	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"overview"},"tableCommand":{"table":"orders","block":"all","start":400,"count":200,"requestSeq":42}}`)
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
