package app

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/agent"
	"github.com/Yacobolo/leapview/internal/dashboard"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
)

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
	page := dashboard.Page{Visuals: []dashboard.PageVisual{
		{ID: "orders-chart", Visual: "orders_chart"},
		{ID: "orders-table", Table: "orders", Title: "Recent orders"},
	}}
	resolved := resolveDashboardTurnReferences([]agent.TurnReference{
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders_chart"}, Name: "Ignore browser title", VisualType: "script"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders"}, Name: "Ignore browser table title"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.secret"}, Name: "Not on page"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "other.orders_chart"}, Name: "Wrong dashboard"},
	}, "executive-sales", page, map[string]reportdef.Visual{
		"orders_chart": {Title: "Orders by status", Type: "bar"},
		"secret":       {Title: "Secret", Type: "line"},
	}, map[string]reportdef.TableVisual{
		"orders": {Title: "Orders", Kind: "table"},
	})

	want := []agent.TurnReference{
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders_chart"}, ComponentID: "orders-chart", VisualID: "orders_chart", Name: "Orders by status", VisualType: "bar"},
		{Reference: agent.TurnReferenceKey{WorkspaceID: "test", Type: "visual", ID: "executive-sales.orders"}, ComponentID: "orders-table", VisualID: "orders", Name: "Recent orders", VisualType: "table"},
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
