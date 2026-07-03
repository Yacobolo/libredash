package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Yacobolo/libredash/internal/analytics/materialize"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/testutil/ssetest"
	uisignals "github.com/Yacobolo/libredash/internal/ui/signals"
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

type canceledTableMetrics struct {
	fakeMetrics
}

type recordingMetrics struct {
	fakeMetrics
	pageIDs []string
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

type failingRefreshAssetMetrics struct {
	emptyPageRuntimeAssetMetrics
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
	return "../../.data/olist"
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
				"orders":       {Title: "Orders", Type: "donut", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: pointInteraction("orders.status", "orders")},
				"ops_pipeline": {Title: "Ops Pipeline", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: pointInteraction("orders.status", "orders")},
			},
			Tables: map[string]reportdef.TableVisual{
				"orders": {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, DefaultSort: dashboard.TableSort{Key: "purchase_date", Direction: "desc"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
			},
			Pages: fakeMetrics{}.Pages(dashboardID),
		}, &semanticmodel.Model{
			Name:  "test",
			Title: "Test Model",
			Tables: map[string]semanticmodel.Table{
				"orders": {
					Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
					Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}, "status": {Expr: "status"}},
				},
			},
			Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Table: "orders", Grain: "order_id", Label: "Orders", Expression: "COUNT(*)"}},
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

func pointInteraction(field string, targets ...string) reportdef.Interaction {
	return reportdef.Interaction{
		PointSelection: reportdef.SelectionInteraction{
			Toggle: true,
			Mappings: []reportdef.SelectionMapping{{
				Field: field,
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
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/operations", nil)
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-app-shell`) || !strings.Contains(body, `<ld-dashboard-page`) {
		t.Fatalf("report page did not render app shell and dashboard route root:\n%s", body)
	}
	if strings.Contains(body, `<ld-report-sidebar`) {
		t.Fatalf("report page still rendered report sidebar:\n%s", body)
	}
	if strings.Contains(body, `<ld-sub-sidebar`) || strings.Contains(body, `<ld-report-canvas`) || strings.Contains(body, `<ld-echart`) || strings.Contains(body, `<ld-report-table`) {
		t.Fatalf("report page rendered dashboard product internals below route root:\n%s", body)
	}
	if !strings.Contains(body, `&#34;compact&#34;:true`) {
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
	if !strings.Contains(decoded, `"tables":{}`) {
		t.Fatalf("operations page should seed no table placeholders:\n%s", decoded)
	}
}

func TestPageRouteSeedsPageScopedFiltersFromURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/overview?state=SP&state=RJ&category=ignored", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/workspaces/test-workspace/dashboards/executive-sales/pages/operations?state=SP&category=ops", nil)
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
		"/workspaces/test-workspace/dashboards/executive-sales/pages/overview",
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
	rendered := html.UnescapeString(body)
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
	if !strings.Contains(rendered, `"id":"chat"`) || !strings.Contains(rendered, `"href":"/chat"`) {
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
	rendered := html.UnescapeString(rec.Body.String())
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

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<ld-login-page`) {
		t.Fatalf("login page did not mount login route root:\n%s", body)
	}
	if !strings.Contains(body, `Sign in with Azure Active Directory`) {
		t.Fatalf("login page did not seed Azure AD provider label:\n%s", body)
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

func (fakeMetrics) RefreshModelTables(_ context.Context, _ string, _ []string) error {
	return nil
}

func (failingRefreshAssetMetrics) RefreshMaterializations(_ context.Context, _ string) error {
	return errors.New("refresh failed")
}

func (failingRefreshAssetMetrics) RefreshModelTables(_ context.Context, _ string, _ []string) error {
	return errors.New("refresh failed")
}

type dependentModelTableMetrics struct {
	fakeMetrics
	refreshed [][]string
}

type localDevStyleModelTableMetrics struct {
	refreshed [][]string
	done      chan []string
}

func (m *localDevStyleModelTableMetrics) Catalog() dashboard.Catalog {
	return fakeMetrics{}.Catalog()
}

func (m *localDevStyleModelTableMetrics) DefaultDashboardID() string {
	return fakeMetrics{}.DefaultDashboardID()
}

func (m *localDevStyleModelTableMetrics) ModelIDForDashboard(dashboardID string) string {
	return fakeMetrics{}.ModelIDForDashboard(dashboardID)
}

func (m *localDevStyleModelTableMetrics) DataDir() string {
	return fakeMetrics{}.DataDir()
}

func (m *localDevStyleModelTableMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	return fakeMetrics{}.Report(dashboardID)
}

func (m *localDevStyleModelTableMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	return fakeMetrics{}.SemanticModel(modelID)
}

func (m *localDevStyleModelTableMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	return fakeMetrics{}.QuerySemantic(ctx, modelID, request)
}

func (m *localDevStyleModelTableMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	return fakeMetrics{}.PreviewSemantic(ctx, modelID, request)
}

func (m *localDevStyleModelTableMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	return fakeMetrics{}.ExecuteDataQuery(ctx, request)
}

func (m *localDevStyleModelTableMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	return fakeMetrics{}.DefaultFilters(dashboardID)
}

func (m *localDevStyleModelTableMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	return fakeMetrics{}.NormalizeTableRequest(dashboardID, request)
}

func (m *localDevStyleModelTableMetrics) Pages(dashboardID string) []dashboard.Page {
	return fakeMetrics{}.Pages(dashboardID)
}

func (m *localDevStyleModelTableMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return fakeMetrics{}.QueryDashboard(ctx, dashboardID, filters)
}

func (m *localDevStyleModelTableMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return fakeMetrics{}.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (m *localDevStyleModelTableMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return fakeMetrics{}.QueryTable(ctx, dashboardID, filters, request)
}

func (m *localDevStyleModelTableMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return fakeMetrics{}.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func (m *localDevStyleModelTableMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	return fakeMetrics{}.RefreshMaterializations(ctx, modelID)
}

func (m *localDevStyleModelTableMetrics) RefreshTables(_ context.Context, modelID string, tableNames []string) error {
	for _, tableName := range tableNames {
		m.refreshed = append(m.refreshed, []string{modelID, tableName})
	}
	if m.done != nil {
		m.done <- append([]string{modelID}, tableNames...)
	}
	return nil
}

func (m *dependentModelTableMetrics) WorkspaceAssets(workspaceID, deploymentID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	catalog, err := testWorkspaceAsset(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), workspace.AssetTypeCatalog, workspaceID, "", "Catalog", "", "catalog.v1", map[string]any{})
	if err != nil {
		return nil, nil, false
	}
	model, err := testWorkspaceAsset(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), workspace.AssetTypeSemanticModel, "olist", catalog.ID, "Olist", "", "semantic_model.v1", map[string]any{})
	if err != nil {
		return nil, nil, false
	}
	orders, err := testWorkspaceAsset(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), workspace.AssetTypeModelTable, "olist.orders", model.ID, "orders", "", "model_table.v1", map[string]any{"PrimaryKey": "order_id", "Source": "orders"})
	if err != nil {
		return nil, nil, false
	}
	summary, err := testWorkspaceAsset(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), workspace.AssetTypeModelTable, "olist.order_summary", model.ID, "order_summary", "", "model_table.v1", map[string]any{"PrimaryKey": "status", "SQL": "SELECT status FROM model.orders"})
	if err != nil {
		return nil, nil, false
	}
	return []workspace.Asset{catalog, model, orders, summary}, []workspace.AssetEdge{
		workspace.NewAssetEdge(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), catalog.ID, model.ID, workspace.AssetEdgeContains),
		workspace.NewAssetEdge(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), model.ID, orders.ID, workspace.AssetEdgeContains),
		workspace.NewAssetEdge(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), model.ID, summary.ID, workspace.AssetEdgeContains),
		workspace.NewAssetEdge(workspace.WorkspaceID(workspaceID), workspace.DeploymentID(deploymentID), summary.ID, orders.ID, workspace.AssetEdgeUsesModelTable),
	}, true
}

func (m *dependentModelTableMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	if modelID != "olist" {
		return fakeMetrics{}.SemanticModel(modelID)
	}
	return &semanticmodel.Model{
		Name:      "olist",
		BaseTable: "order_summary",
		Sources: map[string]semanticmodel.Source{
			"orders": {Path: "orders.csv", Format: "csv"},
		},
		Tables: map[string]semanticmodel.Table{
			"orders":        {Kind: "fact", Source: "orders", PrimaryKey: "order_id"},
			"order_summary": {PrimaryKey: "status", Transform: semanticmodel.Transform{SQL: "SELECT status FROM model.orders"}, ModelDependencies: []string{"orders"}},
		},
	}, true
}

func (m *dependentModelTableMetrics) RefreshModelTables(_ context.Context, _ string, tableNames []string) error {
	m.refreshed = append(m.refreshed, append([]string(nil), tableNames...))
	return nil
}

type failingDependencyModelTableMetrics struct {
	dependentModelTableMetrics
}

func (m *failingDependencyModelTableMetrics) RefreshModelTables(_ context.Context, _ string, tableNames []string) error {
	m.refreshed = append(m.refreshed, append([]string(nil), tableNames...))
	if reflect.DeepEqual(tableNames, []string{"orders"}) {
		return errors.New("dependency failed")
	}
	return nil
}

type missingSemanticModelAssetMetrics struct {
	emptyPageRuntimeAssetMetrics
}

func (m missingSemanticModelAssetMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	if modelID == "olist" {
		return nil, false
	}
	return m.emptyPageRuntimeAssetMetrics.SemanticModel(modelID)
}

func TestUpdatesStreamsDatastarPatchSignals(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/workspaces/test-workspace/updates?dashboard=executive-sales&page=overview&datastar=%7B%22filters%22%3A%7B%22controls%22%3A%7B%22state%22%3A%7B%22type%22%3A%22multi_select%22%2C%22operator%22%3A%22in%22%2C%22values%22%3A%5B%22SP%22%5D%7D%2C%22category%22%3A%7B%22type%22%3A%22text%22%2C%22operator%22%3A%22contains%22%2C%22value%22%3A%22ignored%22%7D%7D%7D%7D", nil)
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
	ssetest.RequirePatchSignal(t, body, func(patch map[string]any) bool {
		status, ok := patch["status"].(map[string]any)
		return ok && status["loading"] == true
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

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/workspaces/test-workspace/updates?dashboard=executive-sales&page=operations&datastar=%7B%22runtime%22%3A%7B%22clientId%22%3A%22test-client%22%2C%22dashboardId%22%3A%22executive-sales%22%2C%22pageId%22%3A%22operations%22%7D%7D", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/refresh-materializations", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestSelectCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"selections":[]},"runtime":{"clientId":"test-client"},"interactionCommand":{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","action":"set","toggle":true,"mappings":[{"field":"orders.status","value":"delivered","label":"delivered"}]},"tableCommand":{"table":"orders","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/select", body)
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
			name: "interaction select",
			path: "/workspaces/test-workspace/commands/select",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"selections":[]},"interactionCommand":{"sourceKind":"visual","sourceId":"ops_pipeline","interactionKind":"point_selection","action":"set","toggle":true,"mappings":[{"field":"orders.status","value":"delivered","label":"delivered"}]},"tableCommand":{"block":"all","start":0,"count":50}}`,
		},
		{
			name: "clear selection",
			path: "/workspaces/test-workspace/commands/clear-selection",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"selections":[{"sourceKind":"visual","sourceId":"ops_pipeline","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","value":"delivered","label":"delivered"}]}]}]},"tableCommand":{"block":"all","start":0,"count":50}}`,
		},
		{
			name: "reset filters",
			path: "/workspaces/test-workspace/commands/reset-filters",
			body: `{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations"},"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"tableCommand":{"block":"all","start":200,"count":50}}`,
		},
		{
			name: "refresh materializations",
			path: "/workspaces/test-workspace/commands/refresh-materializations",
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

func TestDashboardRefreshCommandPersistsMaterializationRun(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "editor@example.com", "Editor", "editor")
	token := testAPIToken(t, ctx, store, principal.ID, "dashboard-refresh")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"operations","modelId":"test"},"filters":{},"tableCommand":{"block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test/commands/refresh-materializations", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	runs, err := repo.ListModelRuns(context.Background(), "test", "test", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != materialize.RunStatusSucceeded || runs[0].ModelID != "test" {
		t.Fatalf("runs = %#v, want one succeeded test model run", runs)
	}
	if runs[0].PrincipalID != principal.ID || runs[0].PrincipalDisplayName != "Editor" {
		t.Fatalf("run attribution = %#v, want Editor principal", runs[0])
	}
}

func TestWorkspaceAssetUpdatesStreamsInitialRefreshState(t *testing.T) {
	store := testStore(t)
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", emptyPageRuntimeAssetMetrics{})
	server := NewWithOptions(emptyPageRuntimeAssetMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	queued, err := repo.CreateRun(context.Background(), materialize.RunInput{WorkspaceID: "test", ModelID: "olist"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := repo.MarkRunSucceeded(context.Background(), "test", queued.ID); err != nil {
		t.Fatalf("mark run succeeded: %v", err)
	}
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/workspaces/test/assets/"+string(assetID)+"/updates?section=refreshes", nil)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	patches := ssetest.PatchSignals(t, body)
	if len(patches) == 0 {
		t.Fatalf("updates did not stream patches:\n%s", body)
	}
	var found bool
	for _, patch := range patches {
		page, ok := patch["page"].(map[string]any)
		if !ok {
			continue
		}
		refresh, ok := page["refresh"].(map[string]any)
		if !ok {
			continue
		}
		runsTable, ok := refresh["runsTable"].(map[string]any)
		if ok && len(runsTable) > 0 {
			found = true
		}
	}
	if !found || !strings.Contains(body, `"status":"succeeded"`) {
		t.Fatalf("updates did not stream succeeded refresh state:\n%s", body)
	}
}

func TestWorkspaceAssetDetailsUpdatesExcludeRefreshesTableAndUnusedRefreshFields(t *testing.T) {
	store := testStore(t)
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", emptyPageRuntimeAssetMetrics{})
	server := NewWithOptions(emptyPageRuntimeAssetMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/workspaces/test/assets/"+string(assetID)+"/updates?section=details", nil)
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

func TestWorkspaceAssetRefreshCommandPublishesRunningAndFinalState(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", emptyPageRuntimeAssetMetrics{})
	principal := testPrincipal(t, ctx, store, "owner@example.com", "Owner", "owner")
	token := testAPIToken(t, ctx, store, principal.ID, "workspace-refresh")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(emptyPageRuntimeAssetMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	updates, unsubscribe := server.broker.Subscribe(workspaceAssetStreamID("test", string(assetID), "details"))
	defer unsubscribe()
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	runs, err := repo.ListModelRuns(context.Background(), "test", "olist", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != materialize.RunStatusSucceeded {
		t.Fatalf("runs = %#v, want one succeeded olist model run", runs)
	}
	if runs[0].PrincipalID != principal.ID || runs[0].PrincipalDisplayName != "Owner" {
		t.Fatalf("run attribution = %#v, want Owner principal", runs[0])
	}
	patches := drainPatches(updates)
	if !patchesContainAssetRefreshStatus(patches, materialize.RunStatusRunning) {
		t.Fatalf("patches did not include running state: %#v", patches)
	}
	if !patchesContainAssetRefreshStatus(patches, materialize.RunStatusSucceeded) {
		t.Fatalf("patches did not include succeeded state: %#v", patches)
	}
}

func TestWorkspaceAssetRefreshCommandPublishesFailedError(t *testing.T) {
	store := testStore(t)
	metrics := failingRefreshAssetMetrics{}
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", metrics)
	auth := testAuth(store, "test", AuthConfig{DevBypass: true})
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	updates, unsubscribe := server.broker.Subscribe(workspaceAssetStreamID("test", string(assetID), "refreshes"))
	defer unsubscribe()
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer dev")
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	runs, err := repo.ListModelRuns(context.Background(), "test", "olist", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != materialize.RunStatusFailed || !strings.Contains(runs[0].Error, "refresh failed") {
		t.Fatalf("runs = %#v, want one failed run with error", runs)
	}
	patches := drainPatches(updates)
	if !patchesContainAssetRefreshStatus(patches, materialize.RunStatusFailed) || !strings.Contains(anyPatchesString(patches), "refresh failed") {
		t.Fatalf("patches did not include failed error state: %#v", patches)
	}
}

func TestWorkspaceModelTableRefreshCommandPersistsDirectAndDependencyRuns(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	metrics := &dependentModelTableMetrics{}
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", metrics)
	principal := testPrincipal(t, ctx, store, "owner@example.com", "Owner", "owner")
	token := testAPIToken(t, ctx, store, principal.ID, "workspace-table-refresh")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeModelTable, "olist.order_summary")
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if got, want := metrics.refreshed, [][]string{{"orders"}, {"order_summary"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshed tables = %#v, want %#v", got, want)
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	rootRuns, err := repo.ListTargetRuns(ctx, "test", materialize.TargetModelTable, "olist.order_summary", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list selected table runs: %v", err)
	}
	if len(rootRuns) != 1 || rootRuns[0].Status != materialize.RunStatusSucceeded || rootRuns[0].TriggerType != materialize.TriggerDirect || rootRuns[0].ParentRunID != "" {
		t.Fatalf("selected table runs = %#v, want direct root run", rootRuns)
	}
	dependencyRuns, err := repo.ListTargetRuns(ctx, "test", materialize.TargetModelTable, "olist.orders", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list dependency table runs: %v", err)
	}
	if len(dependencyRuns) != 1 || dependencyRuns[0].Status != materialize.RunStatusSucceeded || dependencyRuns[0].TriggerType != materialize.TriggerDependency || dependencyRuns[0].ParentRunID != rootRuns[0].ID {
		t.Fatalf("dependency table runs = %#v, want dependency child run", dependencyRuns)
	}
	if rootRuns[0].PrincipalID != principal.ID || dependencyRuns[0].PrincipalID != principal.ID {
		t.Fatalf("principal attribution root=%#v dependency=%#v, want %s", rootRuns[0], dependencyRuns[0], principal.ID)
	}
}

func TestMaterializationRunAPICanExecuteModelTableTargetWithLocalDevRuntimeShape(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "editor@example.com", "Editor", "editor")
	token := testAPIToken(t, ctx, store, principal.ID, "materialization-table-test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	metrics := &localDevStyleModelTableMetrics{done: make(chan []string, 1)}
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/materialization-runs", token, `{"modelId":"olist","targetType":"model_table","targetId":"olist.orders"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID         string `json:"id"`
		TargetID   string `json:"targetId"`
		TargetType string `json:"targetType"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Status != materialize.RunStatusQueued || created.TargetType != materialize.TargetModelTable || created.TargetID != "olist.orders" {
		t.Fatalf("created run = %#v", created)
	}

	select {
	case refreshed := <-metrics.done:
		if !reflect.DeepEqual(refreshed, []string{"olist", "orders"}) {
			t.Fatalf("refreshed = %#v, want olist orders", refreshed)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async model table refresh")
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	run, err := repo.GetRun(ctx, "test", created.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != materialize.RunStatusSucceeded || run.PrincipalID != principal.ID {
		t.Fatalf("run = %#v, want succeeded model table run attributed to editor", run)
	}
}

func TestMaterializationRunAPIMalformedModelTableTargetFailsPersistedRun(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	principal := testPrincipal(t, ctx, store, "editor@example.com", "Editor", "editor")
	token := testAPIToken(t, ctx, store, principal.ID, "materialization-table-invalid-test")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(&localDevStyleModelTableMetrics{}, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})

	createReq := authedJSONRequest(http.MethodPost, "/api/v1/workspaces/test/materialization-runs", token, `{"modelId":"olist","targetType":"model_table","targetId":"other.orders"}`)
	createRec := httptest.NewRecorder()
	server.Routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	repo := materialize.NewSQLRunRepository(store.SQLDB())
	var run materialize.RunRecord
	deadline := time.After(time.Second)
	for {
		var err error
		run, err = repo.GetRun(ctx, "test", created.ID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if run.Status != materialize.RunStatusQueued {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("run remained queued: %#v", run)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if run.Status != materialize.RunStatusFailed || !strings.Contains(run.Error, "does not belong to semantic model") {
		t.Fatalf("run = %#v, want failed wrong-prefix target run", run)
	}
}

func TestWorkspaceSemanticModelRefreshFailsWhenGraphMissing(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	metrics := missingSemanticModelAssetMetrics{}
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", metrics)
	principal := testPrincipal(t, ctx, store, "owner@example.com", "Owner", "owner")
	token := testAPIToken(t, ctx, store, principal.ID, "workspace-semantic-refresh-missing-graph")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	runs, err := repo.ListModelRuns(ctx, "test", "olist", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != materialize.RunStatusFailed || !strings.Contains(runs[0].Error, "unknown semantic model") {
		t.Fatalf("runs = %#v, want failed missing graph run", runs)
	}
}

func TestWorkspaceModelTableRefreshMarksDependencyAndRootFailedWhenDependencyFails(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	metrics := &failingDependencyModelTableMetrics{}
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", metrics)
	principal := testPrincipal(t, ctx, store, "owner@example.com", "Owner", "owner")
	token := testAPIToken(t, ctx, store, principal.ID, "workspace-table-refresh-failure")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeModelTable, "olist.order_summary")
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	rootRuns, err := repo.ListTargetRuns(ctx, "test", materialize.TargetModelTable, "olist.order_summary", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list root runs: %v", err)
	}
	if len(rootRuns) != 1 || rootRuns[0].Status != materialize.RunStatusFailed || !strings.Contains(rootRuns[0].Error, "dependency failed") {
		t.Fatalf("root runs = %#v, want failed selected table run", rootRuns)
	}
	dependencyRuns, err := repo.ListTargetRuns(ctx, "test", materialize.TargetModelTable, "olist.orders", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list dependency runs: %v", err)
	}
	if len(dependencyRuns) != 1 || dependencyRuns[0].Status != materialize.RunStatusFailed || dependencyRuns[0].ParentRunID != rootRuns[0].ID || !strings.Contains(dependencyRuns[0].Error, "dependency failed") {
		t.Fatalf("dependency runs = %#v, want failed dependency linked to root", dependencyRuns)
	}
	if got, want := metrics.refreshed, [][]string{{"orders"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshed tables = %#v, want only failed dependency", got)
	}
}

func TestWorkspaceSemanticModelRefreshCommandPersistsTableChildRuns(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	metrics := &dependentModelTableMetrics{}
	seedActiveDeploymentFromWorkspaceAssets(t, store, "test", metrics)
	principal := testPrincipal(t, ctx, store, "owner@example.com", "Owner", "owner")
	token := testAPIToken(t, ctx, store, principal.ID, "workspace-semantic-refresh")
	auth := testAuth(store, "test", AuthConfig{APITokenOnly: true})
	server := NewWithOptions(metrics, Options{Store: store, Auth: auth, DefaultWorkspaceID: "test"})
	assetID := workspace.NewAssetID(workspace.AssetTypeSemanticModel, "olist")
	path := "/workspaces/test/assets/" + string(assetID) + "/refresh-materializations"
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if got, want := metrics.refreshed, [][]string{{"orders"}, {"order_summary"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshed tables = %#v, want %#v", got, want)
	}
	repo := materialize.NewSQLRunRepository(store.SQLDB())
	modelRuns, err := repo.ListModelRuns(ctx, "test", "olist", materialize.RunPage{Limit: 10})
	if err != nil {
		t.Fatalf("list model runs: %v", err)
	}
	if len(modelRuns) != 1 || modelRuns[0].Status != materialize.RunStatusSucceeded {
		t.Fatalf("model runs = %#v, want succeeded parent run", modelRuns)
	}
	for _, targetID := range []string{"olist.orders", "olist.order_summary"} {
		tableRuns, err := repo.ListTargetRuns(ctx, "test", materialize.TargetModelTable, targetID, materialize.RunPage{Limit: 10})
		if err != nil {
			t.Fatalf("list table runs for %s: %v", targetID, err)
		}
		if len(tableRuns) != 1 || tableRuns[0].TriggerType != materialize.TriggerSemanticModel || tableRuns[0].ParentRunID != modelRuns[0].ID {
			t.Fatalf("table runs for %s = %#v, want semantic model child run", targetID, tableRuns)
		}
	}
}

func drainPatches(ch <-chan map[string]any) []map[string]any {
	var patches []map[string]any
	for {
		select {
		case patch := <-ch:
			patches = append(patches, patch)
		default:
			return patches
		}
	}
}

func patchesContainAssetRefreshStatus(patches []map[string]any, status string) bool {
	for _, patch := range patches {
		refresh, ok := patch["assetRefresh"].(map[string]any)
		if ok && refresh["status"] == status {
			return true
		}
		if page, ok := patch["page"].(uisignals.WorkspaceAssetPageSignal); ok && page.Refresh.Status == status {
			return true
		}
		if page, ok := patch["page"].(map[string]any); ok {
			refresh, ok := page["refresh"].(map[string]any)
			if ok && refresh["status"] == status {
				return true
			}
		}
	}
	return false
}

func anyPatchesString(patches []map[string]any) string {
	bytes, _ := json.Marshal(patches)
	return string(bytes)
}

func TestClearSelectionCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"selections":[{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","value":"delivered","label":"delivered"}]}]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":0,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/clear-selection", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestResetFiltersCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}},"selections":[{"sourceKind":"visual","sourceId":"orders","interactionKind":"point_selection","entries":[{"mappings":[{"field":"orders.status","value":"delivered","label":"delivered"}]}]}]},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"all","start":200,"count":50}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/reset-filters", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandAcceptsDatastarSignals(t *testing.T) {
	body := strings.NewReader(`{"filters":{"controls":{"state":{"type":"multi_select","operator":"in","values":["SP"]}}},"runtime":{"clientId":"test-client"},"tableCommand":{"table":"orders","block":"a","start":400,"count":50,"requestSeq":42,"sort":{"key":"revenue","direction":"desc"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/table-window", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	New(fakeMetrics{}).Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body:\n%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestTableWindowCommandDoesNotPublishCanceledQueries(t *testing.T) {
	server := New(canceledTableMetrics{})
	updates, unsubscribe := server.broker.Subscribe("test-client:executive-sales:overview")
	defer unsubscribe()

	body := strings.NewReader(`{"runtime":{"clientId":"test-client","dashboardId":"executive-sales","pageId":"overview"},"tableCommand":{"table":"orders","block":"all","start":400,"count":50,"requestSeq":42}}`)
	req := httptest.NewRequest(http.MethodPost, "/workspaces/test-workspace/commands/table-window", body)
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
