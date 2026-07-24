package ui

import (
	"encoding/json"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"html"
	"net/url"
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	workspacecompiler "github.com/Yacobolo/leapview/internal/workspace/compiler"
)

func jsonString(value any) string {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func fieldRefs(fields ...string) []reportdef.FieldRef {
	refs := make([]reportdef.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = reportdef.FieldRef{Field: field}
	}
	return refs
}

func TestPageInitialSignalsArePageScoped(t *testing.T) {
	report := reportdef.Dashboard{
		ID:            "report",
		Title:         "Report",
		SemanticModel: "test",
		FilterDefinitions: map[string]dashboardfilter.Definition{
			"state": {
				Label: "State", Field: "orders.state",
				Predicates: []dashboardfilter.PredicatePolicy{{Kind: dashboardfilter.ExpressionSet, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorIn}}},
				Options:    dashboardfilter.OptionSource{Kind: dashboardfilter.OptionSourceDistinct, Limit: 50},
			},
			"category": {
				Label: "Category", Field: "orders.category",
				Predicates: []dashboardfilter.PredicatePolicy{{Kind: dashboardfilter.ExpressionComparison, Operators: []dashboardfilter.Operator{dashboardfilter.OperatorContains}}},
			},
		},
		Visuals: reportdef.MergeVisualizations(reportdef.ChartVisualizations(map[string]reportdef.Visual{
			"active_chart":   {Title: "Active", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Interaction: reportdef.Interaction{PointSelection: reportdef.SelectionInteraction{Mappings: []reportdef.SelectionMapping{{Field: "orders.status", Fact: "orders", Value: "label"}}, Targets: []string{"orders"}}}},
			"active_kpi":     {Type: "kpi", Query: reportdef.VisualQuery{Measures: fieldRefs("order_count")}, Presentation: reportdef.VisualPresentation{Note: "Filtered", Tone: "ink"}},
			"off_page_chart": {Title: "Off Page", Type: "bar", Query: reportdef.VisualQuery{Dimensions: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}},
		}), reportdef.TabularVisualizations("table", map[string]reportdef.TableVisual{
			"orders":   {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, Interaction: reportdef.Interaction{RowSelection: reportdef.SelectionInteraction{Mappings: []reportdef.SelectionMapping{{Field: "orders.order_id", Fact: "orders", Value: "order_id"}}, Targets: []string{"active_chart"}}}, Style: dashboard.TableStyle{Density: "compact", Grid: "full"}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order", Width: 220, Format: "text"}}},
			"off_page": {Title: "Off Page", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
		}), reportdef.TabularVisualizations("matrix", map[string]reportdef.TableVisual{
			"matrix": {Title: "Matrix", Query: reportdef.TableQuery{Rows: fieldRefs("orders.status"), Measures: fieldRefs("order_count")}, Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
		}), reportdef.TabularVisualizations("pivot", map[string]reportdef.TableVisual{
			"pivot": {Title: "Pivot", Query: reportdef.TableQuery{Rows: fieldRefs("orders.status"), Columns: fieldRefs("orders.category"), Measures: fieldRefs("order_count")}, Columns: []dashboard.TableColumn{{Key: "status", Label: "Status"}}},
		})),
		Pages: []dashboard.Page{
			{
				ID:     "showcase",
				Title:  "Showcase",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				FilterBindings: map[string]dashboardfilter.Binding{
					"state": {
						Filter:  "state",
						Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
						URL:     dashboardfilter.URLPolicy{Param: "state", Encoding: dashboardfilter.URLEncodingTypedV1},
					},
				},
				Visuals: []dashboard.PageVisual{
					{ID: "state-slicer", Kind: "slicer", Binding: dashboardfilter.BindingRef{Scope: dashboardfilter.ScopePage, ID: "state"}, Placement: dashboard.PagePlacement{Col: 1, Row: 1, ColSpan: 3, RowSpan: 1}},
					{ID: "kpi", Kind: "visual", Visual: "active_kpi", Placement: dashboard.PagePlacement{Col: 1, Row: 2, ColSpan: 3, RowSpan: 2}},
					{ID: "chart", Kind: "visual", Visual: "active_chart", Placement: dashboard.PagePlacement{Col: 4, Row: 2, ColSpan: 6, RowSpan: 4}},
				},
			},
			{
				ID:     "tables",
				Title:  "Tables",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				FilterBindings: map[string]dashboardfilter.Binding{
					"category": {
						Filter:  "category",
						Default: dashboardfilter.Expression{Kind: dashboardfilter.ExpressionUnfiltered},
						URL:     dashboardfilter.URLPolicy{Param: "category", Encoding: dashboardfilter.URLEncodingTypedV1},
					},
				},
				Visuals: []dashboard.PageVisual{
					{ID: "orders", Kind: "visual", Visual: "orders", Placement: dashboard.PagePlacement{Col: 1, Row: 1, ColSpan: 4, RowSpan: 3}},
					{ID: "matrix", Kind: "visual", Visual: "matrix", Placement: dashboard.PagePlacement{Col: 5, Row: 1, ColSpan: 4, RowSpan: 3}},
					{ID: "pivot", Kind: "visual", Visual: "pivot", Placement: dashboard.PagePlacement{Col: 9, Row: 1, ColSpan: 4, RowSpan: 3}},
				},
			},
		},
	}
	model := &semanticmodel.Model{
		Name:  "test",
		Title: "Test",
		Tables: map[string]semanticmodel.Table{
			"orders": {
				Source: "orders", PrimaryKey: "order_id", Grain: "order_id",
				Dimensions: map[string]semanticmodel.MetricDimension{
					"order_id": {Expr: "order_id", Type: "string"},
					"status":   {Expr: "status", Type: "string"},
					"state":    {Expr: "state", Type: "string"},
					"category": {Expr: "category", Type: "string"},
				},
			},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero", Label: "Orders"}},
	}
	if err := workspacecompiler.ValidateDashboard(&report, map[string]*semanticmodel.Model{"test": model}); err != nil {
		t.Fatal(err)
	}
	definitions, err := workspacecompiler.CompileVisualizationDefinitions(&report, model)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := workspacecompiler.CompileDashboardDefinition(&report, definitions)
	if err != nil {
		t.Fatal(err)
	}

	showcase := renderPageForTest(t, compiled, model, report.Pages[0])
	if !strings.Contains(showcase, `<lv-dashboard-page`) || !strings.Contains(showcase, `data-on:lv-filter-command`) || !strings.Contains(showcase, `data-on:lv-interaction-select`) {
		t.Fatalf("showcase page did not mount dashboard route root with command bridge:\n%s", showcase)
	}
	if strings.Contains(showcase, `data-signals=`) || !strings.Contains(showcase, `data-init="@get('/updates?`) {
		t.Fatalf("showcase page did not render stream-first structural shell:\n%s", showcase)
	}
	for _, attr := range []string{
		` chrome="`, ` page="`, ` filterconfig="`, ` filters="`, ` filteroptions="`, ` visuals="`, ` tables="`, ` status="`,
		`data-attr:chrome`, `data-attr:page`, `data-attr:filterconfig`, `data-attr:filters`, `data-attr:filteroptions`, `data-attr:visuals`, `data-attr:tables`, `data-attr:status`,
	} {
		if strings.Contains(showcase, attr) {
			t.Fatalf("showcase page rendered migrated dashboard bridge attribute %q:\n%s", attr, showcase)
		}
	}
	commandSignalFilters := map[string][]string{
		"data-on:lv-interaction-select":           {"runtime", "interactionCommand"},
		"data-on:lv-interaction-spatial-select":   {"runtime", "spatialInteractionCommand"},
		"data-on:lv-visualization-window-request": {"runtime", "visualWindowCommand"},
		"data-on:lv-visual-spatial-window-change": {"runtime", "visualSpatialWindowCommand"},
		"data-on:lv-filter-command":               {"runtime", "filterCommand"},
		"data-on:lv-filter-options-request":       {"runtime", "filterOptionRequest"},
		"data-on:lv-selection-clear":              {"runtime"},
	}
	for attr, signalPaths := range commandSignalFilters {
		segment := renderedAttrSegment(showcase, attr)
		if !strings.Contains(segment, `filterSignals`) {
			t.Fatalf("%s segment = %q, want command-scoped Datastar signal filter", attr, segment)
		}
		for _, signalPath := range signalPaths {
			if !strings.Contains(segment, signalPath) {
				t.Fatalf("%s segment = %q, want signal path %q", attr, segment, signalPath)
			}
		}
		for _, forbidden := range []string{"visuals", "tables", "filterOptions", "componentStatus"} {
			if strings.Contains(segment, forbidden) {
				t.Fatalf("%s segment = %q, must not post heavy signal %q", attr, segment, forbidden)
			}
		}
	}
	if strings.Contains(showcase, "data-on:lv-visual-window-change") {
		t.Fatalf("showcase page retained the retired table-specific window event:\n%s", showcase)
	}
	agentSegment := renderedAttrSegment(showcase, "data-on:lv-chat-submit")
	for _, expected := range []string{"/chats/turns", "filterSignals", "agent", "agentContext"} {
		if !strings.Contains(agentSegment, expected) {
			t.Fatalf("agent command segment = %q, want %q", agentSegment, expected)
		}
	}
	for _, forbidden := range []string{"visuals", "agentVisuals", "filterOptions", "componentStatus"} {
		if strings.Contains(agentSegment, forbidden) {
			t.Fatalf("agent command segment = %q, must not post heavy signal %q", agentSegment, forbidden)
		}
	}
	agentRestoreSegment := renderedAttrSegment(showcase, "data-on:lv-chat-restore")
	for _, expected := range []string{"/chats/restore", "evt.detail.conversationId", "filterSignals", "agent"} {
		if !strings.Contains(agentRestoreSegment, expected) {
			t.Fatalf("agent restore segment = %q, want %q", agentRestoreSegment, expected)
		}
	}
	for _, forbidden := range []string{"agentContext", "visuals", "agentVisuals", "filterOptions", "componentStatus"} {
		if strings.Contains(agentRestoreSegment, forbidden) {
			t.Fatalf("agent restore segment = %q, must not send unrelated signal %q", agentRestoreSegment, forbidden)
		}
	}
	showcaseSignals := html.UnescapeString(jsonString(BootstrapSignals("client", "stream-instance", dashboard.Catalog{}, compiled, model, definitions, report.Pages, report.Pages[0], dashboard.Filters{})))
	for _, expected := range []string{`"agent":{`, `"agentContext":{`, `"surface":"dashboard"`, `"agentVisuals":{}`} {
		if !strings.Contains(showcaseSignals, expected) {
			t.Fatalf("showcase bootstrap missing dashboard agent signal %s:\n%s", expected, showcaseSignals)
		}
	}
	if !strings.Contains(showcaseSignals, `"active_chart"`) || !strings.Contains(showcaseSignals, `"active_kpi"`) {
		t.Fatalf("showcase bootstrap did not include active chart and KPI visuals:\n%s", showcaseSignals)
	}
	if strings.Contains(showcaseSignals, `"off_page_chart"`) {
		t.Fatalf("showcase bootstrap included off-page chart:\n%s", showcaseSignals)
	}
	if strings.Contains(showcaseSignals, `"kpis"`) {
		t.Fatalf("showcase bootstrap included legacy kpis signal:\n%s", showcaseSignals)
	}
	for _, forbidden := range []string{`"updatesUrl"`, `"routeKey"`, `"csrfToken"`} {
		if strings.Contains(showcaseSignals, forbidden) {
			t.Fatalf("showcase bootstrap leaked %s:\n%s", forbidden, showcaseSignals)
		}
	}
	assertNoDashboardProductDOM(t, showcase)
	if strings.Contains(showcaseSignals, `"tables":`) {
		t.Fatalf("showcase bootstrap included legacy tables signal:\n%s", showcaseSignals)
	}
	if !strings.Contains(showcaseSignals, `"filterContract":{`) || !strings.Contains(showcaseSignals, `"id":"state"`) {
		t.Fatalf("showcase bootstrap did not include the compiled filter contract:\n%s", showcaseSignals)
	}
	if !strings.Contains(showcaseSignals, `"filterState":{`) || !strings.Contains(showcaseSignals, `"appliedControls":{"fb_`) {
		t.Fatalf("showcase bootstrap did not include canonical applied filter state:\n%s", showcaseSignals)
	}
	if !strings.Contains(showcaseSignals, `"id":"category"`) {
		t.Fatalf("showcase bootstrap did not include the dashboard-wide definition catalog:\n%s", showcaseSignals)
	}

	tables := renderPageForTest(t, compiled, model, report.Pages[1])
	tableSignals := html.UnescapeString(jsonString(BootstrapSignals("client", "stream-instance", dashboard.Catalog{}, compiled, model, definitions, report.Pages, report.Pages[1], dashboard.Filters{})))
	for tableID, visualType := range map[string]string{"orders": "table", "matrix": "matrix", "pivot": "pivot"} {
		if !strings.Contains(tableSignals, `"`+tableID+`":{`) || !strings.Contains(tableSignals, `"kind":"`+visualType+`"`) {
			t.Fatalf("visual bootstrap did not include tabular visual %q with type %q:\n%s", tableID, visualType, tableSignals)
		}
	}
	if !strings.Contains(tableSignals, `"presentation":{"rowHeight":28,"striped":true,"showHeader":true}`) || !strings.Contains(tableSignals, `"width":220`) {
		t.Fatalf("tables bootstrap did not include table style and column display metadata:\n%s", tableSignals)
	}
	assertNoDashboardProductDOM(t, tables)
	if !strings.Contains(showcaseSignals, `"interactions":[{"id":"point_selection","kind":"select","mappings":[{"source":{"dataset":"primary","field":"label"},"targetFieldID":"orders.status","targetFactID":"orders"}],"targets":["orders"],"mode":"single","requiresStableIdentity":true}]`) {
		t.Fatalf("showcase bootstrap did not include the compiled point selection:\n%s", showcaseSignals)
	}
	if !strings.Contains(tableSignals, `"interactions":[{"id":"row_selection","kind":"select","mappings":[{"source":{"dataset":"primary","field":"order_id"},"targetFieldID":"orders.order_id","targetFactID":"orders"}],"targets":["active_chart"],"mode":"single","requiresStableIdentity":true}]`) {
		t.Fatalf("tables bootstrap did not include the compiled row selection:\n%s", tableSignals)
	}
	if strings.Contains(tableSignals, `"off_page"`) {
		t.Fatalf("tables bootstrap included off-page table:\n%s", tableSignals)
	}
}

func renderPageForTest(t *testing.T, report dashboarddefinition.Definition, model *semanticmodel.Model, activePage dashboard.Page) string {
	t.Helper()
	var out strings.Builder
	err := Page("client", "", dashboard.Catalog{}, report, model, report.Pages, activePage, dashboard.Filters{}).Render(&out)
	if err != nil {
		t.Fatal(err)
	}
	return html.UnescapeString(out.String())
}

func TestPageCreatesUniqueStreamInstancePerRender(t *testing.T) {
	report := dashboarddefinition.Definition{ID: "report", SemanticModel: "model", Pages: []dashboard.Page{{ID: "overview"}}, Visualizations: map[string]visualizationdefinition.Definition{}}
	model := &semanticmodel.Model{Name: "model"}

	first := renderPageForTest(t, report, model, report.Pages[0])
	second := renderPageForTest(t, report, model, report.Pages[0])
	firstID := queryParamFromRenderedPage(t, first, "streamInstance")
	secondID := queryParamFromRenderedPage(t, second, "streamInstance")
	if firstID == "" || secondID == "" || firstID == secondID {
		t.Fatalf("stream instances = %q and %q, want unique non-empty ids", firstID, secondID)
	}
}

func queryParamFromRenderedPage(t *testing.T, body, name string) string {
	t.Helper()
	start := strings.Index(body, "/updates?")
	if start < 0 {
		t.Fatalf("rendered page has no updates URL")
	}
	end := strings.IndexAny(body[start:], "'\"")
	if end < 0 {
		t.Fatalf("rendered page has unterminated updates URL")
	}
	raw := strings.ReplaceAll(body[start:start+end], "&amp;", "&")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse updates URL: %v", err)
	}
	return parsed.Query().Get(name)
}

func renderedAttrSegment(body, name string) string {
	prefix := name + `="`
	start := strings.Index(body, prefix)
	if start < 0 {
		return ""
	}
	valueStart := start + len(prefix)
	valueEnd := strings.Index(body[valueStart:], `"`)
	if valueEnd < 0 {
		return body[start:]
	}
	return body[start : valueStart+valueEnd+1]
}

func assertNoDashboardProductDOM(t *testing.T, body string) {
	t.Helper()
	for _, tag := range []string{
		"lv-sub-sidebar",
		"lv-report-canvas",
		"lv-filter-panel",
		"lv-filter-card",
		"lv-kpi-card",
		"lv-echart",
		"lv-report-table",
		"lv-report-footer",
		"lv-visual-modal",
	} {
		if strings.Contains(body, "<"+tag) {
			t.Fatalf("Go rendered dashboard product DOM <%s> below route root:\n%s", tag, body)
		}
	}
}
