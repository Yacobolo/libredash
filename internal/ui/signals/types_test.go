package signals

import (
	"strings"
	"testing"

	"github.com/Yacobolo/leapview/internal/agent"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
)

func TestChatTranscriptItemsProjectsTurnReferences(t *testing.T) {
	items := ChatTranscriptItems([]agent.ChatTranscriptItem{{
		ID: "user_1", Kind: "user", Text: "Explain this", References: []agent.TurnReference{{
			Reference: agent.TurnReferenceKey{WorkspaceID: "sales", Type: "visual", ID: "executive-sales.revenue"},
			Name:      "Revenue by month", Workspace: agent.TurnReferenceWorkspace{ID: "sales", Name: "Sales"},
			Hierarchy: []string{"Sales", "Executive Sales", "Overview"}, Href: "/overview", VisualType: "line",
		}},
	}})

	if len(items) != 1 || items[0].References == nil || len(*items[0].References) != 1 {
		t.Fatalf("turn reference signal = %#v", items)
	}
	reference := (*items[0].References)[0]
	if reference.Name != "Revenue by month" || reference.Reference.Type != "visual" || reference.Href != "/overview" {
		t.Fatalf("turn reference signal = %#v", reference)
	}
	if reference.VisualType == nil || *reference.VisualType != "line" {
		t.Fatalf("turn reference visual type = %#v, want line", reference.VisualType)
	}
}

func TestDashboardInitialEnvelopeValidatesPageScopedPayloads(t *testing.T) {
	report := testDashboardReport()
	model := testSemanticModel()
	envelope := DashboardInitialEnvelope("client", "stream-instance", dashboard.Catalog{}, report, model, report.Pages, report.Pages[0], dashboard.Filters{})

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
	if envelope.Runtime.StreamInstanceID == nil || *envelope.Runtime.StreamInstanceID != "stream-instance" {
		t.Fatalf("stream instance id = %#v", envelope.Runtime.StreamInstanceID)
	}
	if envelope.Status.RefreshID != "" || envelope.Status.Generation != 0 {
		t.Fatalf("initial refresh status = %#v", envelope.Status)
	}
	if len(envelope.ComponentStatus) != 0 {
		t.Fatalf("initial component status = %#v, want empty", envelope.ComponentStatus)
	}
	if envelope.AgentContext.Surface != "dashboard" || envelope.AgentContext.PageID != report.Pages[0].ID || envelope.AgentContext.ModelID != model.Name {
		t.Fatalf("agent context = %#v", envelope.AgentContext)
	}
	if envelope.AgentContext.References == nil || envelope.AgentVisuals == nil {
		t.Fatalf("dashboard agent collections must be non-nil: context=%#v visuals=%#v", envelope.AgentContext, envelope.AgentVisuals)
	}
}

func TestDashboardEnvelopeRejectsMissingReferencedPayload(t *testing.T) {
	report := testDashboardReport()
	envelope := DashboardInitialEnvelope("client", "stream-instance", dashboard.Catalog{}, report, testSemanticModel(), report.Pages, report.Pages[0], dashboard.Filters{})
	delete(envelope.Visuals, "active_chart")

	err := ValidateDashboardEnvelope(envelope)
	if err == nil || !strings.Contains(err.Error(), `missing visual "active_chart"`) {
		t.Fatalf("validate error = %v", err)
	}
}

func TestDashboardEnvelopeRejectsUnusedPayload(t *testing.T) {
	report := testDashboardReport()
	envelope := DashboardInitialEnvelope("client", "stream-instance", dashboard.Catalog{}, report, testSemanticModel(), report.Pages, report.Pages[0], dashboard.Filters{})
	envelope.Visuals["off_page_chart"] = DashboardVisualFromDashboard(dashboard.Visual{ID: "off_page_chart", Type: "bar"})

	err := ValidateDashboardEnvelope(envelope)
	if err == nil || !strings.Contains(err.Error(), `unused visual payload "off_page_chart"`) {
		t.Fatalf("validate error = %v", err)
	}
}

func TestInteractionSignalPreservesSelectionScope(t *testing.T) {
	got := interactionSignal("point_selection", reportdef.SelectionInteraction{
		Toggle: true,
		Mappings: []reportdef.SelectionMapping{
			{Field: "activity_date", Grain: "month", Value: "label"},
			{Field: "ratings.rating_bucket", Fact: "ratings", Value: "series"},
		},
		Targets: []string{"activity_by_month"},
	})

	if !got.Toggle || len(got.Mappings) != 2 {
		t.Fatalf("interaction signal = %#v", got)
	}
	if got.Mappings[0].Fact != "" || got.Mappings[0].Grain != "month" {
		t.Fatalf("conformed mapping = %#v", got.Mappings[0])
	}
	if got.Mappings[1].Fact != "ratings" || got.Mappings[1].Grain != "" {
		t.Fatalf("fact-local mapping = %#v", got.Mappings[1])
	}
}

func TestChatInitialEnvelopeValidates(t *testing.T) {
	envelope := ChatInitialEnvelope(dashboard.Catalog{}, "test", "", "list", testChatViewState(ChatSignal{
		ActiveConversationID: "",
		Conversations:        []ChatConversationSummary{},
		Transcript:           nil,
		Status:               ChatStatus{Enabled: true},
		Composer:             ComposerSignal{Placeholder: "Ask"},
	}))

	if err := ValidateChatEnvelope(envelope); err != nil {
		t.Fatalf("validate chat envelope: %v", err)
	}
	if envelope.Chrome.Sidebar.PrimaryAction == nil || envelope.Chrome.Sidebar.PrimaryAction.Href != "/chats/new" {
		t.Fatalf("chat primary action = %#v", envelope.Chrome.Sidebar.PrimaryAction)
	}
	if envelope.Chrome.Sidebar.History == nil {
		t.Fatalf("chat history missing: %#v", envelope.Chrome.Sidebar)
	}
	if envelope.Page.View != "list" {
		t.Fatalf("chat page view = %q", envelope.Page.View)
	}
	if envelope.AgentContext.Surface != "chat" || envelope.AgentContext.WorkspaceID != "test" || envelope.AgentContext.References == nil {
		t.Fatalf("chat context = %#v", envelope.AgentContext)
	}
	if envelope.AgentReferenceSearch.Results == nil {
		t.Fatalf("chat reference search = %#v", envelope.AgentReferenceSearch)
	}
	if envelope.Chrome.Sidebar.History.Label != "Chats" {
		t.Fatalf("chat history search config = %#v", envelope.Chrome.Sidebar.History)
	}
}

func TestChatInitialEnvelopeOnlyListActivatesChatNav(t *testing.T) {
	list := ChatInitialEnvelope(dashboard.Catalog{}, "test", "", "list", testChatViewState(ChatSignal{
		ActiveConversationID: "",
		Conversations:        []ChatConversationSummary{},
		Transcript:           nil,
		Status:               ChatStatus{Enabled: true},
		Composer:             ComposerSignal{Placeholder: "Ask"},
	}))
	if list.Chrome.Sidebar.Active != "chat" {
		t.Fatalf("list chat sidebar active = %q, want chat", list.Chrome.Sidebar.Active)
	}

	draft := ChatInitialEnvelope(dashboard.Catalog{}, "test", "", "new", testChatViewState(ChatSignal{
		ActiveConversationID: "",
		Conversations:        []ChatConversationSummary{},
		Transcript:           nil,
		Status:               ChatStatus{Enabled: true},
		Composer:             ComposerSignal{Placeholder: "Ask"},
	}))
	if draft.Chrome.Sidebar.Active != "" {
		t.Fatalf("draft chat sidebar active = %q, want none", draft.Chrome.Sidebar.Active)
	}

	conversation := ChatInitialEnvelope(dashboard.Catalog{}, "test", "", "conversation", testChatViewState(ChatSignal{
		ActiveConversationID: "agentconv_1",
		Conversations: []ChatConversationSummary{{
			ID:    "agentconv_1",
			Title: "Conversation",
		}},
		Transcript: nil,
		Status:     ChatStatus{Enabled: true},
		Composer:   ComposerSignal{Placeholder: "Ask"},
	}))
	if conversation.Chrome.Sidebar.Active != "" {
		t.Fatalf("conversation chat sidebar active = %q, want none", conversation.Chrome.Sidebar.Active)
	}
	if len(conversation.Chrome.Sidebar.History.Items) != 1 || !conversation.Chrome.Sidebar.History.Items[0].Active {
		t.Fatalf("conversation history item not active: %#v", conversation.Chrome.Sidebar.History.Items)
	}
}

func testChatViewState(signal ChatSignal) ChatViewState {
	return ChatViewState{
		Agent:   signal,
		Visuals: map[string]DashboardVisual{},
	}
}

func TestCatalogSidebarUsesGlobalChat(t *testing.T) {
	sidebar := SidebarConfigForCatalog(dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "operations", Title: "Operations"},
	})

	item, ok := sidebarItem(sidebar, "chat")
	if !ok {
		t.Fatalf("catalog sidebar missing chat item: %#v", sidebar.Groups)
	}
	if item.Href != "/chats" {
		t.Fatalf("catalog chat href = %q, want global chat", item.Href)
	}
}

func TestWorkspaceSidebarUsesGlobalChat(t *testing.T) {
	sidebar := SidebarConfigForWorkspace(dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "operations", Title: "Operations"},
	}, "workspaces", "Viewer")

	item, ok := sidebarItem(sidebar, "chat")
	if !ok {
		t.Fatalf("workspace sidebar missing chat item: %#v", sidebar.Groups)
	}
	if item.Href != "/chats" {
		t.Fatalf("chat href = %q, want global chat", item.Href)
	}
}

func TestSidebarWorkspaceTitleDoesNotInventDefaultWorkspace(t *testing.T) {
	global := SidebarConfigForCatalog(dashboard.Catalog{})
	if global.WorkspaceTitle != "LeapView" {
		t.Fatalf("global workspace title = %q, want app title", global.WorkspaceTitle)
	}

	workspace := SidebarConfigForWorkspace(dashboard.Catalog{
		Workspace: dashboard.CatalogWorkspace{ID: "operations"},
	}, "workspaces", "Viewer")
	if workspace.WorkspaceTitle != "operations" {
		t.Fatalf("workspace title = %q, want workspace id fallback", workspace.WorkspaceTitle)
	}
}

func sidebarItem(sidebar SidebarSignal, id string) (SidebarItemSignal, bool) {
	for _, group := range sidebar.Groups {
		for _, item := range group.Items {
			if item.ID == id {
				return item, true
			}
		}
	}
	return SidebarItemSignal{}, false
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
			"orders": {Source: "orders", PrimaryKey: "order_id", Grain: "order_id", Dimensions: map[string]semanticmodel.MetricDimension{"order_id": {Expr: "order_id"}, "status": {Expr: "status"}, "state": {Expr: "state"}, "category": {Expr: "category"}}},
		},
		Measures: map[string]semanticmodel.MetricMeasure{"order_count": {Fact: "orders", Aggregation: "count", Empty: "zero", Label: "Orders"}},
	}
}

func testFieldRefs(fields ...string) []reportdef.FieldRef {
	refs := make([]reportdef.FieldRef, len(fields))
	for i, field := range fields {
		refs[i] = reportdef.FieldRef{Field: field}
	}
	return refs
}
