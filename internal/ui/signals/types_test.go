package signals

import (
	"strings"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

func TestDashboardInitialEnvelopeValidatesPageScopedPayloads(t *testing.T) {
	report := testDashboardReport()
	model := testSemanticModel()
	envelope := DashboardInitialEnvelope(".data", "client", "", dashboard.Catalog{}, report, model, report.Pages, report.Pages[0], dashboard.Filters{})

	if err := ValidateDashboardEnvelope(envelope); err != nil {
		t.Fatalf("validate dashboard envelope: %v", err)
	}
	if _, ok := envelope.Visuals["active_chart"]; !ok {
		t.Fatalf("active visual missing: %#v", envelope.Visuals)
	}
	if _, ok := envelope.Visuals["off_page_chart"]; ok {
		t.Fatalf("off-page visual was emitted: %#v", envelope.Visuals)
	}
	if _, ok := envelope.Filters.Controls["state"]; !ok {
		t.Fatalf("page filter control missing: %#v", envelope.Filters)
	}
	if _, ok := envelope.Filters.Controls["category"]; ok {
		t.Fatalf("off-page filter control was emitted: %#v", envelope.Filters)
	}
}

func TestDashboardEnvelopeRejectsMissingReferencedPayload(t *testing.T) {
	report := testDashboardReport()
	envelope := DashboardInitialEnvelope(".data", "client", "", dashboard.Catalog{}, report, testSemanticModel(), report.Pages, report.Pages[0], dashboard.Filters{})
	delete(envelope.Visuals, "active_chart")

	err := ValidateDashboardEnvelope(envelope)
	if err == nil || !strings.Contains(err.Error(), `missing visual "active_chart"`) {
		t.Fatalf("validate error = %v", err)
	}
}

func TestDashboardEnvelopeRejectsUnusedPayload(t *testing.T) {
	report := testDashboardReport()
	envelope := DashboardInitialEnvelope(".data", "client", "", dashboard.Catalog{}, report, testSemanticModel(), report.Pages, report.Pages[0], dashboard.Filters{})
	envelope.Visuals["off_page_chart"] = dashboard.Visual{ID: "off_page_chart"}

	err := ValidateDashboardEnvelope(envelope)
	if err == nil || !strings.Contains(err.Error(), `unused visual payload "off_page_chart"`) {
		t.Fatalf("validate error = %v", err)
	}
}

func TestChatInitialEnvelopeValidates(t *testing.T) {
	envelope := ChatInitialEnvelope(dashboard.Catalog{}, "csrf", "", ChatSignal{
		ActiveConversationID: "",
		Conversations:        []ChatConversationSummary{},
		Transcript:           nil,
		Status:               ChatStatus{Enabled: true},
		Composer:             ComposerSignal{Placeholder: "Ask"},
	})

	if err := ValidateChatEnvelope(envelope); err != nil {
		t.Fatalf("validate chat envelope: %v", err)
	}
	if envelope.Page.Sidebar.Items[0].Href != "/chat/new" {
		t.Fatalf("chat sidebar = %#v", envelope.Page.Sidebar)
	}
}

func testDashboardReport() reportdef.Dashboard {
	return reportdef.Dashboard{
		ID:            "report",
		Title:         "Report",
		SemanticModel: "test",
		Filters: map[string]reportdef.FilterDefinition{
			"state":    {Type: "multi_select", Label: "State", Dimension: "orders.state", URLParam: "state", Operator: "in"},
			"category": {Type: "text", Label: "Category", Dimension: "orders.category", URLParam: "category", DefaultOperator: "contains"},
		},
		Visuals: map[string]reportdef.Visual{
			"active_chart":   {Title: "Active", Type: "bar", Query: reportdef.VisualQuery{Dimensions: testFieldRefs("orders.status"), Measures: testFieldRefs("order_count")}},
			"off_page_chart": {Title: "Off Page", Type: "bar", Query: reportdef.VisualQuery{Dimensions: testFieldRefs("orders.status"), Measures: testFieldRefs("order_count")}},
		},
		Tables: map[string]reportdef.TableVisual{
			"orders": {Title: "Orders", Query: reportdef.TableQuery{Table: "orders", Fields: []string{"orders.order_id"}}, Columns: []dashboard.TableColumn{{Key: "order_id", Label: "Order"}}},
		},
		Pages: []dashboard.Page{
			{
				ID:     "overview",
				Title:  "Overview",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				Visuals: []dashboard.PageVisual{
					{ID: "state-filter", Kind: "filter_card", Filter: "state", X: 0, Y: 0, Width: 100, Height: 40},
					{ID: "chart", Kind: "bar_chart", Visual: "active_chart", X: 0, Y: 48, Width: 100, Height: 100},
				},
			},
			{
				ID:     "detail",
				Title:  "Detail",
				Canvas: dashboard.PageCanvas{Width: 1200, Height: 800},
				Visuals: []dashboard.PageVisual{
					{ID: "orders", Kind: "table", Table: "orders", X: 0, Y: 0, Width: 100, Height: 100},
				},
			},
		},
	}
}

func testSemanticModel() *semanticmodel.Model {
	return &semanticmodel.Model{
		Name:  "test",
		Title: "Test",
		Tables: map[string]semanticmodel.Table{
			"orders": {Kind: "fact", Source: "orders", PrimaryKey: "order_id", Grain: "order_id", Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}, "status": {Expr: "status"}, "state": {Expr: "state"}, "category": {Expr: "category"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Table: "orders", Grain: "order_id", Label: "Orders", Expression: "COUNT(*)"}},
	}
}

func testFieldRefs(fields ...string) []reportdef.FieldRef {
	refs := make([]reportdef.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = reportdef.FieldRef{Field: field}
	}
	return refs
}
