package app

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	productsearch "github.com/Yacobolo/leapview/internal/search"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

func TestAgentReferenceSignalIncludesSearchVisualSubtype(t *testing.T) {
	result := agentReferenceSignal(productsearch.Result{
		Reference:  productsearch.Reference{WorkspaceID: "sales", Type: productsearch.TypeVisual, ID: "orders.revenue"},
		Name:       "Revenue",
		VisualType: "line",
		Workspace:  productsearch.Workspace{ID: "sales", Name: "Sales"},
		Locations:  []productsearch.Location{},
		Context:    []productsearch.ContextTag{},
	})
	if result.VisualType == nil || *result.VisualType != "line" {
		t.Fatalf("agent reference visual subtype = %#v", result)
	}
}

func TestResolveChatTurnReferencesUsesAuthorizedSearchMetadata(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "test", servingstate.DefaultEnvironment, "Orders dashboard", "Warehouse")
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	resolved, err := server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{DevAuthBypass: true}, agent.TurnContext{
		Surface:     "chat",
		WorkspaceID: "test",
		References: []agent.TurnReference{{
			Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "dashboard", ID: "dev-dashboard"},
			Name:      "Untrusted browser title", ModelID: "wrong-model",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.References) != 1 {
		t.Fatalf("resolved references = %#v", resolved.References)
	}
	ref := resolved.References[0]
	if ref.Reference.Type != "dashboard" || ref.Reference.ID != "dev-dashboard" || ref.Name != "Orders dashboard" {
		t.Fatalf("resolved reference trusted browser metadata: %#v", ref)
	}
}

func TestResolveChatTurnReferencesRejectsNonAttachableTypes(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "test", servingstate.DefaultEnvironment, "Orders dashboard", "Warehouse")
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	resolved, err := server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{DevAuthBypass: true}, agent.TurnContext{
		Surface: "chat",
		References: []agent.TurnReference{
			{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "source", ID: "dev.orders"}},
			{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "dashboard", ID: "dev-dashboard"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.References) != 1 || resolved.References[0].Reference.Type != "dashboard" {
		t.Fatalf("resolved non-attachable context = %#v", resolved.References)
	}
}

func TestResolveChatTurnReferencesAppliesCredentialToReferenceWorkspace(t *testing.T) {
	store := testStore(t)
	seedEnvironmentAssetDeployment(t, store, "test", servingstate.DefaultEnvironment, "Orders dashboard", "Warehouse")
	server := NewWithOptions(fakeMetrics{}, Options{Store: store, DefaultWorkspaceID: "test"})
	resolved, err := server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{
		DevAuthBypass: true,
		Credential: agent.CredentialScope{
			WorkspaceID: "test",
			Privileges:  []string{string(access.PrivilegeViewItem)},
			Restricted:  true,
		},
	}, agent.TurnContext{
		Surface: "chat",
		References: []agent.TurnReference{{
			Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "dashboard", ID: "dev-dashboard"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.References) != 1 || resolved.WorkspaceID != "test" {
		t.Fatalf("resolved context = %#v", resolved)
	}

	_, err = server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{
		DevAuthBypass: true,
		Credential: agent.CredentialScope{
			WorkspaceID: "other",
			Privileges:  []string{string(access.PrivilegeViewItem)},
			Restricted:  true,
		},
	}, agent.TurnContext{
		Surface: "chat",
		References: []agent.TurnReference{{
			Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "dashboard", ID: "dev-dashboard"},
		}},
	})
	if err == nil {
		t.Fatal("foreign workspace credential resolved referenced context")
	}
}

func TestResolveAgentTurnContextRejectsExcessReferences(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	references := make([]agent.TurnReference, agent.MaxTurnReferences+1)
	for index := range references {
		references[index] = agent.TurnReference{
			Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "measure", ID: "test.order_count"},
		}
	}
	_, err := server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{DevAuthBypass: true}, agent.TurnContext{
		Surface: "chat", References: references,
	})
	if err == nil {
		t.Fatal("excess references were silently truncated")
	}
}

func TestResolveDashboardTurnReferencesUsesCompiledMetadata(t *testing.T) {
	page := dashboard.Page{ID: "overview", Title: "Overview", Visuals: []dashboard.PageVisual{
		{ID: "orders-chart", Visual: "orders_chart"},
		{ID: "orders-table", Table: "orders", Title: "Recent orders"},
	}}
	resolved := resolveDashboardTurnReferences([]agent.TurnReference{
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders_chart"}, Name: "Ignore browser title", VisualType: "script", Href: "javascript:alert(1)", Hierarchy: []string{"Forged"}},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders"}, Name: "Ignore browser table title"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.secret"}, Name: "Not on page"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "other.orders_chart"}, Name: "Wrong dashboard"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "other", Type: "visual", ID: "executive-sales.orders_chart"}, Name: "Wrong workspace"},
	}, dashboardTurnReferenceContext{
		Workspace:   agent.TurnReferenceWorkspace{ID: "test", Name: "Test workspace"},
		DashboardID: "executive-sales", DashboardTitle: "Executive Sales", Page: page,
	}, map[string]reportdef.Visual{
		"orders_chart": {Title: "Orders by status", Type: "bar"},
		"secret":       {Title: "Secret", Type: "line"},
	}, map[string]reportdef.TableVisual{
		"orders": {Title: "Orders", Kind: "table"},
	})

	wantReference := func(id, componentID, visualID, name, visualType string) agent.TurnReference {
		href := "/workspaces/test/dashboards/executive-sales/pages/overview"
		return agent.TurnReference{
			Reference:   agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: id},
			ComponentID: componentID, VisualID: visualID, Name: name, VisualType: visualType,
			Workspace: agent.TurnReferenceWorkspace{ID: "test", Name: "Test workspace"},
			Hierarchy: []string{"Test workspace", "Executive Sales", "Overview"}, Href: href,
			Locations: []agent.TurnReferenceLocation{{DashboardID: "executive-sales", DashboardName: "Executive Sales", PageID: "overview", PageName: "Overview", Href: href}},
			Context:   []string{"current_page", "current_dashboard", "current_workspace"},
		}
	}
	want := []agent.TurnReference{
		wantReference("executive-sales.orders_chart", "orders-chart", "orders_chart", "Orders by status", "bar"),
		wantReference("executive-sales.orders", "orders-table", "orders", "Recent orders", "table"),
	}
	if len(resolved) != len(want) {
		t.Fatalf("resolved references = %#v, want %#v", resolved, want)
	}
	for index := range want {
		if !reflect.DeepEqual(resolved[index], want[index]) {
			t.Fatalf("resolved[%d] = %#v, want %#v", index, resolved[index], want[index])
		}
	}
}
