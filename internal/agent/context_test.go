package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTurnContextItemsIncludeResolvedWorkspaceReferences(t *testing.T) {
	items := turnContextItems(&TurnContext{
		Surface:     "chat",
		WorkspaceID: "sales",
		References: []TurnReference{{
			Reference: TurnReferenceKey{WorkspaceID: "sales", Type: "measure", ID: "orders.order_count"},
			Name:      "Order count",
			Workspace: TurnReferenceWorkspace{ID: "sales", Name: "Sales"},
			ModelID:   "sales",
			DatasetID: "orders",
			FieldID:   "order_count",
		}},
	})
	if len(items) != 1 || items[0].Key != "leapview_context" {
		t.Fatalf("context items = %#v", items)
	}
	payload, err := json.Marshal(items[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	input := string(payload)

	for _, want := range []string{`"surface":"chat"`, `"type":"measure"`, "Order count"} {
		if !strings.Contains(input, want) {
			t.Fatalf("contextual input missing %q:\n%s", want, input)
		}
	}
}

func TestTurnContextNormalizationKeepsSameReferenceIDAcrossWorkspaces(t *testing.T) {
	normalized := (TurnContext{
		Surface: "chat",
		References: []TurnReference{
			{Reference: TurnReferenceKey{WorkspaceID: "sales", Type: "field", ID: "orders.revenue"}},
			{Reference: TurnReferenceKey{WorkspaceID: "visuals", Type: "field", ID: "orders.revenue"}},
		},
	}).normalized()

	if got := len(normalized.References); got != 2 {
		t.Fatalf("normalized references = %#v, want two workspace-qualified references", normalized.References)
	}
}
