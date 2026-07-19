package app

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	materializesqlite "github.com/Yacobolo/libredash/internal/analytics/materialize/sqlite"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/dashboard/consumer"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/testutil/ssetest"
	"github.com/Yacobolo/libredash/internal/workspace"
	workspacesqlite "github.com/Yacobolo/libredash/internal/workspace/sqlite"
)

func fieldRefs(fields ...string) []reportdef.FieldRef {
	refs := make([]reportdef.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = reportdef.FieldRef{Field: field}
	}
	return refs
}

type fakeMetrics struct{}

func (fakeMetrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	for _, target := range request.Targets {
		switch target.Kind {
		case consumer.KindVisual:
			visual, err := fakeMetrics{}.QueryVisualPage(ctx, request.DashboardID, request.PageID, request.Filters, target.ID)
			publish(consumer.Result{Target: target, Visual: visual, Err: err})
		case consumer.KindFilterOptions:
			options, err := fakeMetrics{}.QueryFilterOptionsPage(ctx, request.DashboardID, request.PageID, []string{target.ID})
			publish(consumer.Result{Target: target, FilterOptions: options, Err: err})
		case consumer.KindTable:
			table, err := fakeMetrics{}.QueryTablePage(ctx, request.DashboardID, request.PageID, request.Filters, target.TableRequest)
			publish(consumer.Result{Target: target, Table: table, Err: err})
		}
	}
	return ctx.Err()
}

type canceledTableMetrics struct {
	fakeMetrics
}

type recordingMetrics struct {
	fakeMetrics
	pageIDs chan string
}

func (m *recordingMetrics) ExecuteConsumersPage(ctx context.Context, request consumer.Request, publish consumer.Publisher) error {
	for range request.Targets {
		m.pageIDs <- request.PageID
	}
	return m.fakeMetrics.ExecuteConsumersPage(ctx, request, publish)
}

type namedWorkspaceMetrics struct {
	fakeMetrics
	workspaceID string
	dashboardID string
	title       string
}

func (m namedWorkspaceMetrics) Catalog() dashboard.Catalog {
	return dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: m.workspaceID, Title: m.workspaceID},
		Models:    []dashboard.CatalogModel{{ID: "test", Title: "Test Model"}},
		Dashboards: []dashboard.CatalogDashboard{{
			ID:            m.dashboardID,
			Title:         m.title,
			SemanticModel: "test",
			PageCount:     1,
		}},
	}
}

func (m namedWorkspaceMetrics) DefaultDashboardID() string {
	return m.dashboardID
}

func (m namedWorkspaceMetrics) Pages(dashboardID string) []dashboard.Page {
	if dashboardID != m.dashboardID {
		return nil
	}
	return []dashboard.Page{{ID: "overview", Title: "Overview"}}
}

func (m namedWorkspaceMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if dashboardID != m.dashboardID {
		return reportdef.Dashboard{}, nil, false
	}
	return reportdef.Dashboard{
		ID:            m.dashboardID,
		Title:         m.title,
		SemanticModel: "test",
		Pages:         m.Pages(dashboardID),
	}, &semanticmodel.Model{Name: "test", Title: "Test Model"}, true
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

func (fakeMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	if dashboardID != "executive-sales" {
		return reportdef.Dashboard{}, nil, false
	}
	return reportdef.Dashboard{
			ID:            "executive-sales",
			Title:         "Executive Sales Dashboard",
			SemanticModel: "test",
			Filters: map[string]reportdef.FilterDefinition{
				"state":    {Type: "multi_select", Label: "State", Dimension: "orders.status", URLParam: "state", Operator: "in", Values: reportdef.FilterValues{Source: "distinct", Limit: 50}},
				"category": {Type: "text", Label: "Category", Dimension: "orders.status", URLParam: "category", DefaultOperator: "contains", Operators: []string{"contains", "equals"}},
			},
			Visuals: map[string]reportdef.Visual{
				"orders":       {Title: "Orders", Type: "donut", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: pointInteraction("orders.status", "orders", "ops_pipeline")},
				"ops_pipeline": {Title: "Ops Pipeline", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: pointInteraction("orders.status", "orders", "ops_pipeline")},
			},
			Tables: map[string]reportdef.TableVisual{
				"order_rows": {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, DefaultSort: dashboard.TableSort{Key: "purchase_date", Direction: "desc"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
			},
			Pages: fakeMetrics{}.Pages(dashboardID),
		}, &semanticmodel.Model{
			Name:  "test",
			Title: "Test Model",
			Tables: map[string]semanticmodel.Table{
				"orders": {
					Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
					Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id", Type: "string"}, "status": {Expr: "status", Type: "string"}},
				},
			},
			Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero", Label: "Orders"}},
		}, true
}

func (fakeMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	_, model, ok := fakeMetrics{}.Report("executive-sales")
	if !ok || model.Name != modelID {
		return nil, false
	}
	return model, true
}

func (fakeMetrics) QuerySemantic(_ context.Context, _ string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	rows := reportdef.QueryRows{
		{"status": "delivered", "order_count": 42},
		{"status": "shipped", "order_count": 7},
	}
	return rows[:min(len(rows), request.Limit)], nil
}

func (fakeMetrics) PreviewSemantic(_ context.Context, _ string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	rows := reportdef.QueryRows{
		{"order_id": "o1", "status": "delivered"},
		{"order_id": "o2", "status": "shipped"},
	}
	return rows[:min(len(rows), request.Limit)], nil
}

func (fakeMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	switch request.Kind {
	case dataquery.KindSemanticAggregate:
		rows, err := fakeMetrics{}.QuerySemantic(ctx, request.ModelID, reportdef.AggregateQuery{
			Table:      request.Target,
			Dimensions: dataFieldsToReportFields(request.Fields),
			Measures:   dataFieldsToReportFields(request.Measures),
			Time:       reportdef.QueryTime{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
			Filters:    dataFiltersToReportFilters(request.Filters),
			Sort:       dataSortToReportSort(request.Sort),
			Limit:      request.Limit,
			Offset:     request.Offset,
		})
		return fakeDataQueryResult(rows, request.IncludeTotal), err
	case dataquery.KindSemanticRows:
		rows, err := fakeMetrics{}.PreviewSemantic(ctx, request.ModelID, reportdef.RowQuery{
			Table:      request.Target,
			Dimensions: dataFieldsToReportFields(request.Fields),
			Measures:   dataFieldsToReportFields(request.Measures),
			Filters:    dataFiltersToReportFilters(request.Filters),
			Sort:       dataSortToReportSort(request.Sort),
			Limit:      request.Limit,
			Offset:     request.Offset,
		})
		return fakeDataQueryResult(rows, request.IncludeTotal), err
	case dataquery.KindSourceRows, dataquery.KindModelTableRows:
		return dataquery.Result{
			Columns:        dataquery.ColumnsFromNames([]string{"order_id", "status"}),
			Rows:           []dataquery.Row{{"order_id": "o1", "status": "delivered"}, {"order_id": "o2", "status": "shipped"}},
			TotalRows:      2,
			TotalRowsKnown: request.IncludeTotal,
			SQL:            string(request.Kind) + ": " + request.Target,
		}, nil
	default:
		return dataquery.Result{}, fmt.Errorf("unsupported data query kind %q", request.Kind)
	}
}

func fakeDataQueryResult(rows reportdef.QueryRows, includeTotal bool) dataquery.Result {
	out := make([]dataquery.Row, 0, len(rows))
	columnSet := map[string]bool{}
	columns := []string{}
	for _, row := range rows {
		converted := dataquery.Row{}
		for key, value := range row {
			converted[key] = value
			if !columnSet[key] {
				columnSet[key] = true
				columns = append(columns, key)
			}
		}
		out = append(out, converted)
	}
	return dataquery.Result{Columns: dataquery.ColumnsFromNames(columns), Rows: out, TotalRows: len(out), TotalRowsKnown: includeTotal}
}

func dataFieldsToReportFields(fields []dataquery.Field) []reportdef.QueryField {
	out := make([]reportdef.QueryField, 0, len(fields))
	for _, field := range fields {
		out = append(out, reportdef.QueryField{Field: field.Field, Alias: field.Alias})
	}
	return out
}

func dataFiltersToReportFilters(filters []dataquery.Filter) []reportdef.QueryFilter {
	out := make([]reportdef.QueryFilter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]reportdef.QueryFilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, reportdef.QueryFilterGroup{Filters: dataFiltersToReportFilters(group.Filters)})
		}
		out = append(out, reportdef.QueryFilter{Field: filter.Field, Operator: filter.Operator, Values: append([]any{}, filter.Values...), Groups: groups})
	}
	return out
}

func dataSortToReportSort(sort []dataquery.Sort) []reportdef.QuerySort {
	out := make([]reportdef.QuerySort, 0, len(sort))
	for _, item := range sort {
		out = append(out, reportdef.QuerySort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func (fakeMetrics) ExplainSemanticQuery(_ string, request reportdef.AggregateQuery) (semanticquery.Plan, error) {
	return semanticquery.NewPlanner(fakeMetrics{}.mustSemanticModel()).Plan(reportdef.SemanticAggregateRequest(request))
}

func (fakeMetrics) ExplainSemanticPreview(_ string, request reportdef.RowQuery) (semanticquery.Plan, error) {
	return semanticquery.NewPlanner(fakeMetrics{}.mustSemanticModel()).PlanRows(reportdef.SemanticRowRequest(request))
}

func (fakeMetrics) mustSemanticModel() *semanticmodel.Model {
	_, model, _ := fakeMetrics{}.Report("executive-sales")
	return model
}

func (fakeMetrics) DefaultFilters(_ string) dashboard.Filters {
	return dashboard.Filters{
		Controls: map[string]dashboard.FilterControl{
			"state":    {Type: "multi_select", Operator: "in", Values: []string{}},
			"category": {Type: "text", Operator: "contains"},
		},
		Selections: []dashboard.InteractionSelection{},
	}
}

func pointInteraction(field, fact string, targets ...string) reportdef.Interaction {
	return reportdef.Interaction{
		PointSelection: reportdef.SelectionInteraction{
			Toggle: true,
			Mappings: []reportdef.SelectionMapping{{
				Field: field,
				Fact:  fact,
				Value: "label",
				Label: "label",
			}},
			Targets: targets,
		},
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
				{ID: "orders-table", Kind: "table", Table: "order_rows", X: 0, Y: 160, Width: 100, Height: 100},
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
			Loading:     false,
			LastUpdated: "12:00:00",
		},
		Visuals: map[string]dashboard.Visual{
			chartID: {ID: chartID, Type: "bar", Shape: "category_value", Title: chartTitle, Unit: "orders", Data: []dashboard.Datum{{"label": "delivered", "value": 1}}},
		},
	}, nil
}

func (fakeMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	patch, err := fakeMetrics{}.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	visual, ok := patch.Visuals[visualID]
	if !ok {
		visual = dashboard.Visual{ID: visualID, Type: "bar", Shape: "category_value", Title: visualID}
	}
	return visual, err
}

func (fakeMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	patch, err := fakeMetrics{}.QueryDashboardPage(ctx, dashboardID, pageID, filters)
	visuals := make(map[string]dashboard.Visual, len(visualIDs))
	for _, id := range visualIDs {
		visual, ok := patch.Visuals[id]
		if !ok {
			visual = dashboard.Visual{ID: id, Type: "bar", Shape: "category_value", Title: id}
		}
		visuals[id] = visual
	}
	return visuals, err
}

func (fakeMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	patch, err := fakeMetrics{}.QueryDashboardPage(ctx, dashboardID, pageID, dashboard.Filters{})
	options := make(map[string][]dashboard.FilterOption, len(filterIDs))
	for _, id := range filterIDs {
		options[id] = patch.FilterOptions[id]
	}
	return options, err
}

func (m *recordingMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	m.pageIDs <- pageID
	return m.fakeMetrics.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (m *recordingMetrics) QueryVisualPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualID string) (dashboard.Visual, error) {
	m.pageIDs <- pageID
	return m.fakeMetrics.QueryVisualPage(ctx, dashboardID, pageID, filters, visualID)
}

func (m *recordingMetrics) QueryVisualsPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, visualIDs []string) (map[string]dashboard.Visual, error) {
	m.pageIDs <- pageID
	return m.fakeMetrics.QueryVisualsPage(ctx, dashboardID, pageID, filters, visualIDs)
}

func (m *recordingMetrics) QueryFilterOptionsPage(ctx context.Context, dashboardID, pageID string, filterIDs []string) (map[string][]dashboard.FilterOption, error) {
	m.pageIDs <- pageID
	return m.fakeMetrics.QueryFilterOptionsPage(ctx, dashboardID, pageID, filterIDs)
}

func TestPageRouteRendersRequestedYamlPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/operations", nil)
	rec := httptest.NewRecorder()

	server := New(fakeMetrics{})
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := renderedWithBootstrap(t, server, rec.Body.String(), "")
	if !strings.Contains(body, `<ld-app-shell`) || !strings.Contains(body, `<ld-dashboard-page`) {
		t.Fatalf("report page did not render app shell and dashboard route root:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("report page still rendered report sidebar:\n%s", body)
	}
	if strings.Contains(body, `<ld-sub-sidebar`) || strings.Contains(body, `<ld-report-canvas`) || strings.Contains(body, `<ld-echart`) || strings.Contains(body, `<ld-report-table`) {
		t.Fatalf("report page rendered dashboard product internals below route root:\n%s", body)
	}
	if !strings.Contains(body, `"compact":true`) {
		t.Fatalf("report page did not compact the primary sidebar:\n%s", body)
	}
	if !strings.Contains(body, `/workspaces/test-workspace/dashboards/executive-sales/pages/operations`) {
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
	if strings.Contains(decoded, `"order_rows"`) {
		t.Fatalf("operations page should seed no off-page tabular visuals:\n%s", decoded)
	}
}

func TestPageRouteSeedsPageScopedFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/overview?state=SP&state=RJ&category=ignored", nil)
	rec := httptest.NewRecorder()

	server := New(fakeMetrics{})
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := renderedWithBootstrap(t, server, rec.Body.String(), "")
	if !strings.Contains(body, `/static/url-sync.js`) {
		t.Fatalf("page did not include url sync script:\n%s", body)
	}
	if !strings.Contains(body, `"state":["RJ","SP"]`) {
		t.Fatalf("page did not seed state url params:\n%s", body)
	}
	if !strings.Contains(body, `"values":["RJ","SP"]`) {
		t.Fatalf("page did not seed state filter values:\n%s", body)
	}
	if strings.Contains(body, `"category"`) {
		t.Fatalf("overview page seeded off-page category filter:\n%s", body)
	}
}

func TestPageRouteSeedsOperationsPageFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/operations?state=SP&category=ops", nil)
	rec := httptest.NewRecorder()

	server := New(fakeMetrics{})
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := renderedWithBootstrap(t, server, rec.Body.String(), "")
	if !strings.Contains(body, `"category":"ops"`) && !strings.Contains(body, `"value":"ops"`) {
		t.Fatalf("operations page did not seed category URL filter:\n%s", body)
	}
	if strings.Contains(body, `"state":{"type"`) || strings.Contains(body, `"urlParams":{"state"`) || strings.Contains(body, `"urlParamShape":{"state"`) {
		t.Fatalf("operations page seeded off-page state filter:\n%s", body)
	}
}

func TestHTMLRoutesIncludeSelfHostedDatastarRuntimeAndDevInspector(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "")
	for _, path := range []string{
		"/login",
		"/",
		"/workspaces/test-workspace/dashboards/executive-sales/pages/overview",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			assertDevDatastarRuntime(t, rec.Body.String())
		})
	}
}

func TestHTMLRoutesOmitDatastarInspectorInProduction(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "1")
	for _, path := range []string{
		"/login",
		"/",
		"/workspaces/test-workspace/dashboards/executive-sales/pages/overview",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `/static/vendor/datastar-1.0.2.js?v=`) {
				t.Fatalf("page missing self-hosted Datastar runtime:\n%s", body)
			}
			for _, notWant := range []string{
				`/static/datastar-inspector.js`,
				`<datastar-inspector`,
			} {
				if strings.Contains(body, notWant) {
					t.Fatalf("production page included dev inspector marker %q:\n%s", notWant, body)
				}
			}
			if strings.Contains(body, "cdn.jsdelivr.net") {
				t.Fatalf("page references CDN-hosted Datastar runtime:\n%s", body)
			}
		})
	}
}

func TestHTMLRoutesHonorConfiguredStaticAssetVersion(t *testing.T) {
	t.Setenv("LIBREDASH_ASSET_VERSION", "prod-build-123")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `/static/app-shell.js?v=prod-build-123`) {
		t.Fatalf("home route missing configured static asset version:\n%s", body)
	}
	if strings.Contains(body, `?v=dev`) {
		t.Fatalf("home route leaked development static asset version:\n%s", body)
	}
}

func TestStaticAssetsCacheOnlyCurrentVersionedURLs(t *testing.T) {
	t.Chdir("../..")
	t.Setenv("LIBREDASH_PRODUCTION", "")
	t.Setenv("LIBREDASH_ASSET_VERSION", "prod-build-123")
	handler := New(fakeMetrics{}).Routes()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "current version",
			path: "/static/login-background-loader.js?v=prod-build-123",
			want: "public, max-age=31536000, immutable",
		},
		{
			name: "stale version",
			path: "/static/login-background-loader.js?v=old-build",
			want: "no-store",
		},
		{
			name: "unversioned",
			path: "/static/login-background-loader.js",
			want: "no-store",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Fatalf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}

	t.Setenv("LIBREDASH_ASSET_VERSION", "")
	req := httptest.NewRequest(http.MethodGet, "/static/login-background-loader.js?v=dev", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dev version status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("dev version Cache-Control = %q, want no-store", got)
	}
}

func TestStaticAssetCacheHeaderClasses(t *testing.T) {
	t.Setenv("LIBREDASH_PRODUCTION", "")
	t.Setenv("LIBREDASH_ASSET_VERSION", "prod-build-123")
	handler := staticAssetCache(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "current versioned asset",
			path: "/static/app.css?v=prod-build-123",
			want: "public, max-age=31536000, immutable",
		},
		{
			name: "hashed chunk asset",
			path: "/static/chunks/shared-app-shell-sv895r5c.js",
			want: "public, max-age=31536000, immutable",
		},
		{
			name: "font asset",
			path: "/static/files/inter-latin-wght-normal.woff2",
			want: "public, max-age=86400",
		},
		{
			name: "unversioned entrypoint",
			path: "/static/app-shell.js",
			want: "no-store",
		},
		{
			name: "stale versioned asset",
			path: "/static/app.css?v=old-build",
			want: "no-store",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if got := rec.Header().Get("Cache-Control"); got != tc.want {
				t.Fatalf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}
}

func assertDevDatastarRuntime(t *testing.T, body string) {
	t.Helper()
	for _, want := range []string{
		`/static/vendor/datastar-1.0.2.js?v=dev`,
		`/static/datastar-inspector.js`,
		`<datastar-inspector`,
		`signals-url="/__dev/pagestream/signals"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing Datastar inspector marker %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "cdn.jsdelivr.net") {
		t.Fatalf("page references CDN-hosted Datastar runtime:\n%s", body)
	}
}

func TestHomeRouteRendersDashboardCatalog(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server := New(fakeMetrics{})
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := renderedWithBootstrap(t, server, rec.Body.String(), "")
	rendered := body
	if !strings.Contains(rendered, `<ld-app-shell`) || !strings.Contains(rendered, `<ld-catalog-page`) {
		t.Fatalf("home did not mount catalog route root:\n%s", rendered)
	}
	if !strings.Contains(rendered, `/static/catalog-page.js`) {
		t.Fatalf("home missing catalog route bundle:\n%s", rendered)
	}
	if !strings.Contains(rendered, `Dashboards`) {
		t.Fatalf("home missing dashboard catalog title:\n%s", body)
	}
	if !strings.Contains(rendered, `Executive Sales Dashboard`) {
		t.Fatalf("home missing dashboard card:\n%s", body)
	}
	if !strings.Contains(rendered, `"href":"/workspaces/test-workspace/dashboards/executive-sales"`) {
		t.Fatalf("home missing dashboard link:\n%s", body)
	}
	for _, want := range []string{`Dashboards`, `/`, `Workspaces`, `/workspaces`, `Connections`, `/connections`, `Admin`, `/admin`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("home sidebar missing %q:\n%s", want, body)
		}
	}
	for _, notWant := range []string{`Metric Views`, `/metrics`, `Semantic Models`, `/models`, `Settings`, `/workspaces/test-workspace/permissions`, `/workspaces/test-workspace/chat`} {
		if strings.Contains(rendered, notWant) {
			t.Fatalf("home sidebar rendered removed navigation %q:\n%s", notWant, body)
		}
	}
	if !strings.Contains(rendered, `"id":"chat"`) || !strings.Contains(rendered, `"href":"/chats"`) {
		t.Fatalf("home sidebar did not render global chat navigation:\n%s", body)
	}
	if strings.Contains(rendered, `<ld-sub-sidebar`) {
		t.Fatalf("dashboard catalog should not render sub sidebar:\n%s", body)
	}
}

func TestHomeRouteAggregatesDBBackedWorkspaceCatalogs(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	workspaceRepo := workspacesqlite.NewRepository(store.SQLDB())
	for _, row := range []workspace.EnsureInput{
		{ID: "operations", Title: "Operations Workspace"},
		{ID: "sales", Title: "Sales Workspace"},
		{ID: "visuals", Title: "Visuals Workspace"},
	} {
		if err := workspaceRepo.Ensure(ctx, row); err != nil {
			t.Fatalf("ensure workspace: %v", err)
		}
	}
	metrics := NewMultiWorkspaceMetrics("operations", map[string]QueryMetrics{
		"operations": namedWorkspaceMetrics{workspaceID: "operations", dashboardID: "fulfillment-operations", title: "Fulfillment Operations"},
		"sales":      namedWorkspaceMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales"},
		"visuals":    namedWorkspaceMetrics{workspaceID: "visuals", dashboardID: "visual-showcase", title: "Visual Showcase"},
	})
	server := NewWithOptions(metrics, Options{Store: store, WorkspaceRepo: workspaceRepo, DefaultWorkspaceID: "operations"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	rendered := renderedWithBootstrap(t, server, rec.Body.String(), "")
	for _, want := range []string{
		`Fulfillment Operations`,
		`Executive Sales`,
		`Visual Showcase`,
		`"href":"/workspaces/operations/dashboards/fulfillment-operations"`,
		`"href":"/workspaces/sales/dashboards/executive-sales"`,
		`"href":"/workspaces/visuals/dashboards/visual-showcase"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("home catalog missing %q:\n%s", want, rendered)
		}
	}
}

func TestLoginRouteRendersAzureADLogin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	server := New(fakeMetrics{})
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := renderedWithBootstrap(t, server, rec.Body.String(), "")
	if !strings.Contains(body, `<ld-login-page`) {
		t.Fatalf("login page did not mount login route root:\n%s", body)
	}
	if !strings.Contains(body, `background-module-src="/static/topology-background.js`) {
		t.Fatalf("login page did not seed versioned background module src on route root:\n%s", body)
	}
	if !strings.Contains(body, `Sign in with Azure Active Directory`) {
		t.Fatalf("login page did not seed Azure AD provider label:\n%s", body)
	}
	if strings.Contains(body, `data-init__delay`) || strings.Contains(body, `libredash-login-background-init`) {
		t.Fatalf("login page still uses Datastar for lazy background init:\n%s", body)
	}
	if !strings.Contains(body, `/static/login-background-loader.js`) {
		t.Fatalf("login page did not load the CSP-compatible background loader asset:\n%s", body)
	}
	if strings.Contains(body, `requestIdleCallback`) {
		t.Fatalf("login page rendered background loader inline instead of from static asset:\n%s", body)
	}
	if !strings.Contains(body, `/static/topology-background.js`) {
		t.Fatalf("login page did not include lazy topology background asset:\n%s", body)
	}
	if strings.Contains(body, `starfederation/datastar`) || strings.Contains(body, `cdn.jsdelivr`) {
		t.Fatalf("login page still references remote Datastar runtime:\n%s", body)
	}
	if !strings.Contains(body, `/static/vendor/datastar-1.0.2.js?v=dev`) {
		t.Fatalf("login page did not include framework Datastar runtime:\n%s", body)
	}
}

func TestDashboardRouteRedirectsToFirstPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if got := rec.Header().Get("Location"); got != "/workspaces/test-workspace/dashboards/executive-sales/pages/overview" {
		t.Fatalf("Location = %q, want first page", got)
	}
}

func TestServerDoesNotResolveBlankWorkspaceToDefault(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{DefaultWorkspaceID: "test"})

	if got := server.workspaceID(""); got != "" {
		t.Fatalf("workspaceID(\"\") = %q, want blank", got)
	}
	if _, ok := server.metricsForWorkspace(""); ok {
		t.Fatal("metricsForWorkspace(\"\") returned metrics, want no implicit workspace")
	}
}

func TestWorkspaceScopedDashboardRoutesRejectCrossWorkspaceLookup(t *testing.T) {
	metrics := NewMultiWorkspaceMetrics("sales", map[string]QueryMetrics{
		"sales":      namedWorkspaceMetrics{workspaceID: "sales", dashboardID: "executive-sales", title: "Executive Sales"},
		"operations": namedWorkspaceMetrics{workspaceID: "operations", dashboardID: "fulfillment-operations", title: "Fulfillment Operations"},
	})
	server := NewWithOptions(metrics, Options{DefaultWorkspaceID: "sales"})

	okReq := httptest.NewRequest(http.MethodGet, "/workspaces/operations/dashboards/fulfillment-operations/pages/overview", nil)
	okRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("operations route status = %d, want 200; body:\n%s", okRec.Code, okRec.Body.String())
	}

	crossReq := httptest.NewRequest(http.MethodGet, "/workspaces/operations/dashboards/executive-sales/pages/overview", nil)
	crossRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(crossRec, crossReq)
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace route status = %d, want 404; body:\n%s", crossRec.Code, crossRec.Body.String())
	}
}

func TestUnknownPageRouteReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/missing", nil)
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
		Cardinality:   dashboard.ExactCardinality(1),
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

func (canceledTableMetrics) ExecuteConsumersPage(_ context.Context, request consumer.Request, publish consumer.Publisher) error {
	for _, target := range request.Targets {
		publish(consumer.Result{Target: target, Err: context.Canceled})
	}
	return nil
}

func TestUpdatesStreamsDatastarPatchSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?route=dashboard&workspace=test-workspace&dashboard=executive-sales&page=overview&state=SP&category=ignored", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	body := rec.Body.String()
	patches := ssetest.PatchSignals(t, body)
	if len(patches) == 0 {
		t.Fatalf("body does not contain Datastar patch signal event:\n%s", body)
	}
	firstStatus, ok := patches[0]["status"].(map[string]any)
	if !ok || firstStatus["loading"] != true {
		t.Fatalf("first patch status = %#v, want loading=true; patches=%#v", firstStatus, patches)
	}
	ssetest.RequirePatchSignal(t, body, func(patch map[string]any) bool {
		status, ok := patch["status"].(map[string]any)
		return ok && status["loading"] == false
	})
	ssetest.RequirePatchSignal(t, body, func(patch map[string]any) bool {
		filters, ok := patch["filters"].(map[string]any)
		if !ok {
			return false
		}
		controls, ok := filters["controls"].(map[string]any)
		if !ok {
			return false
		}
		state, ok := controls["state"].(map[string]any)
		if !ok {
			return false
		}
		values, ok := state["values"].([]any)
		return ok && len(values) == 1 && values[0] == "SP"
	})
	for _, patch := range patches {
		filters, ok := patch["filters"].(map[string]any)
		if !ok {
			continue
		}
		controls, ok := filters["controls"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := controls["category"]; ok {
			t.Fatalf("patch streamed off-page category filter: %#v", patch)
		}
	}
}

func TestUpdatesStreamsPageScopedChartSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?route=dashboard&workspace=test-workspace&dashboard=executive-sales&page=operations&datastar=%7B%22runtime%22%3A%7B%22clientId%22%3A%22test-client%22%2C%22dashboardId%22%3A%22executive-sales%22%2C%22pageId%22%3A%22operations%22%7D%7D", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"visuals":{"ops_pipeline"`) {
		t.Fatalf("updates did not stream active page chart:\n%s", body)
	}
	if strings.Contains(body, `"visuals":{"orders"`) {
		t.Fatalf("updates streamed off-page chart:\n%s", body)
	}
	if strings.Contains(body, `"order_rows"`) {
		t.Fatalf("updates should not stream off-page tabular visuals:\n%s", body)
	}
	if strings.Contains(body, `"kpis"`) {
		t.Fatalf("updates streamed legacy KPI signal:\n%s", body)
	}
}

func TestDashboardRefreshCommandRouteIsRemoved(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"visualWindowCommand":{"visual":"order_rows","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/refresh", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestSelectCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"selections":[]},"runtime":{"clientId":"test-client"},"interactionCommand":{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","action":"set","toggle":true,"mappings":[{"field":"orders.status","fact":"orders","value":"delivered","label":"delivered"}]},"visualWindowCommand":{"visual":"order_rows","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/select", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	assertDatastarCommandAccepted(t, rec)
}

func assertDatastarCommandAccepted(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	if got := rec.Body.String(); got != "{}\n" {
		t.Fatalf("body = %q, want empty Datastar JSON signal patch", got)
	}
}

func TestPageCommandsQueryActivePage(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		queries int
	}{
		{
			name:    "interaction select",
			path:    "/workspaces/test-workspace/commands/select",
			body:    `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"selections":[]},"interactionCommand":{"sourceKind":"visual","sourceId":"ops_pipeline","interactionKind":"point_selection","action":"set","toggle":true,"mappings":[{"field":"orders.status","fact":"orders","value":"delivered","label":"delivered"}]},"visualWindowCommand":{"block":"all","start":0,"count":50}}`,
			queries: 1,
		},
		{
			name: "clear selection",
			path: "/workspaces/test-workspace/commands/clear-selection",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"selections":[{"sourceKind":"visual","sourceId":"ops_pipeline","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","fact":"orders","value":"delivered","label":"delivered"}]}]}]},"visualWindowCommand":{"block":"all","start":0,"count":50}}`,
		},
		{
			name:    "reload",
			path:    "/workspaces/test-workspace/commands/reload",
			body:    `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"visualWindowCommand":{"block":"all","start":200,"count":50}}`,
			queries: 2,
		},
		{
			name:    "reset filters",
			path:    "/workspaces/test-workspace/commands/reset-filters",
			body:    `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"visualWindowCommand":{"block":"all","start":200,"count":50}}`,
			queries: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &recordingMetrics{pageIDs: make(chan string, 4)}
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			New(metrics).Routes().ServeHTTP(rec, req)

			assertDatastarCommandAccepted(t, rec)
			for i := 0; i < tt.queries; i++ {
				select {
				case pageID := <-metrics.pageIDs:
					if pageID != "operations" {
						t.Fatalf("queried page ID = %q, want operations", pageID)
					}
				case <-time.After(time.Second):
					t.Fatalf("timed out after %d/%d targeted page queries", i, tt.queries)
				}
			}
		})
	}
}

func TestDashboardRefreshCommandDoesNotPersistRefreshRun(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "editor@example.com", "Editor", "editor")
	token := testAPIToken(t, ctx, store, principal.ID, "dashboard-refresh")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations","modelId":"test"},"filters":{},"visualWindowCommand":{"block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test/commands/refresh", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	repo := materializesqlite.NewSQLRunRepository(store.SQLDB())
	runs, err := repo.ListRuns(context.Background(), "test", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %#v, want none for removed dashboard refresh command", runs)
	}
}

func TestWorkspaceAssetDetailsUpdatesExcludeRefreshesTableAndUnusedRefreshFields(t *testing.T) {
	store := testStore(t)
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", emptyPageRuntimeAssetMetrics{})
	server := NewWithOptions(emptyPageRuntimeAssetMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/updates?route=workspace_asset&workspace=test&asset="+string(assetID)+"&section=details", nil)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	patches := ssetest.PatchSignals(t, body)
	if len(patches) == 0 {
		t.Fatalf("details updates did not stream patches:\n%s", body)
	}
	for _, patch := range patches {
		if _, ok := patch["assetRefreshesTable"]; ok {
			t.Fatalf("details updates streamed refreshes table: %#v", patch["assetRefreshesTable"])
		}
		page, ok := patch["page"].(map[string]any)
		if !ok {
			continue
		}
		refresh, ok := page["refresh"].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := refresh["runsTable"]; ok {
			t.Fatalf("details updates streamed refreshes table: %#v", refresh["runsTable"])
		}
		for _, key := range []string{"error", "lastAttempt", "lastDuration"} {
			if _, ok := refresh[key]; ok {
				t.Fatalf("details assetRefresh included unused field %q: %#v", key, refresh)
			}
		}
	}
}

func TestLegacySemanticModelRefreshRouteIsRemoved(t *testing.T) {
	store := testStore(t)
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", emptyPageRuntimeAssetMetrics{})
	server := NewWithOptions(emptyPageRuntimeAssetMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test/assets/"+string(assetID)+"/refresh", nil)
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("legacy semantic-model refresh status = %d, want 404", rec.Code)
	}
}

func TestClearSelectionCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"selections":[{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","fact":"orders","value":"delivered","label":"delivered"}]}]}]},"runtime":{"clientId":"test-client"},"visualWindowCommand":{"visual":"order_rows","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/clear-selection", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	assertDatastarCommandAccepted(t, rec)
}

func TestResetFiltersCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"selections":[{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","fact":"orders","value":"delivered","label":"delivered"}]}]}]},"runtime":{"clientId":"test-client"},"visualWindowCommand":{"visual":"order_rows","block":"all","start":200,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/reset-filters", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	assertDatastarCommandAccepted(t, rec)
}

func TestTableWindowCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"visualWindowCommand":{"visual":"order_rows","block":"a","start":400,"count":50,"requestSeq":42,"sort":{"key":"revenue","direction":"desc"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/visual-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	assertDatastarCommandAccepted(t, rec)
}

func TestTableWindowCommandDoesNotPublishCanceledQueries(t *testing.T) {
	server := New(canceledTableMetrics{})
	updates, unsubscribe := server.broker.Subscribe("test-client:executive-sales:overview")
	defer unsubscribe()

	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"overview"},"visualWindowCommand":{"visual":"order_rows","block":"all","start":400,"count":50,"requestSeq":42}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/visual-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	assertDatastarCommandAccepted(t, rec)
	deadline := time.After(time.Second)
	for {
		select {
		case patch := <-updates:
			if _, ok := patch["tables"]; ok {
				t.Fatalf("received canceled table payload: %#v", patch)
			}
			if statuses, ok := patch["componentStatus"].(map[string]any); ok {
				if status, ok := statuses["visual:orders"].(map[string]any); ok && status["error"] != "" {
					t.Fatalf("cancellation surfaced as target error: %#v", patch)
				}
			}
			if status, ok := patch["status"].(map[string]any); ok && status["loading"] == false {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for canceled table generation to complete")
		}
	}
}
