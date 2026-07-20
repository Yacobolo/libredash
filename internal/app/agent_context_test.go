package app

import (
	"net/http/httptest"
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agent"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
)

func TestResolveChatTurnReferencesUsesAuthorizedSearchMetadata(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
	resolved, err := server.resolveAgentTurnContext(httptest.NewRequest("GET", "/chats/new", nil), agent.Scope{DevAuthBypass: true}, agent.TurnContext{
		Surface:     "chat",
		WorkspaceID: "test",
		References: []agent.TurnReference{{
			Kind: "measure", ID: "test.order_count", Title: "Untrusted browser title", WorkspaceID: "test", ModelID: "wrong-model",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.References) != 1 {
		t.Fatalf("resolved references = %#v", resolved.References)
	}
	ref := resolved.References[0]
	if ref.Kind != "measure" || ref.ID != "test.order_count" || ref.Title == "Untrusted browser title" || ref.ModelID != "test" {
		t.Fatalf("resolved reference trusted browser metadata: %#v", ref)
	}
}

func TestResolveChatTurnReferencesAppliesCredentialToReferenceWorkspace(t *testing.T) {
	server := NewWithOptions(fakeMetrics{}, Options{Store: testStore(t), DefaultWorkspaceID: "test"})
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
			Kind: "measure", ID: "test.order_count", WorkspaceID: "test",
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
			Kind: "measure", ID: "test.order_count", WorkspaceID: "test",
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
			Kind: "measure", ID: "test.order_count", WorkspaceID: "test",
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
		{Kind: "visual", ComponentID: "orders-chart", VisualID: "orders_chart", Title: "Ignore browser title", VisualType: "script"},
		{Kind: "visual", ComponentID: "orders-table", VisualID: "orders", Title: "Ignore browser table title"},
		{Kind: "visual", ComponentID: "off-page", VisualID: "secret", Title: "Not on page"},
	}, page, map[string]reportdef.Visual{
		"orders_chart": {Title: "Orders by status", Type: "bar"},
		"secret":       {Title: "Secret", Type: "line"},
	}, map[string]reportdef.TableVisual{
		"orders": {Title: "Orders", Kind: "table"},
	})

	want := []agent.TurnReference{
		{Kind: "visual", ComponentID: "orders-chart", VisualID: "orders_chart", Title: "Orders by status", VisualType: "bar"},
		{Kind: "visual", ComponentID: "orders-table", VisualID: "orders", Title: "Recent orders", VisualType: "table"},
	}
	if len(resolved) != len(want) {
		t.Fatalf("resolved references = %#v, want %#v", resolved, want)
	}
	for index := range want {
		if resolved[index] != want[index] {
			t.Fatalf("resolved[%d] = %#v, want %#v", index, resolved[index], want[index])
		}
	}
}
