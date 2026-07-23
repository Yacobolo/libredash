package tools

import (
	"slices"
	"testing"

	"github.com/Yacobolo/toolbelt/apigen/runtime/agenttool"
)

func TestAPIGenOperationsUseGeneratedReadOnlyToolContracts(t *testing.T) {
	operations := APIGenOperations()
	if len(operations) != 2 {
		t.Fatalf("APIGenOperations() count = %d, want 2", len(operations))
	}
	operationsByName := make(map[string]APIGenOperation, len(operations))
	for _, operation := range operations {
		operationsByName[operation.Tool.Name] = operation
		if operation.Tool.Effect != agenttool.EffectRead {
			t.Fatalf("tool %q effect = %q, want read", operation.Tool.Name, operation.Tool.Effect)
		}
		if operation.Tool.OperationID != operation.Contract.OperationID {
			t.Fatalf("tool %q operation = %q, registry operation = %q", operation.Tool.Name, operation.Tool.OperationID, operation.Contract.OperationID)
		}
	}
	for name, operationID := range map[string]string{
		"query_semantic_model":   "querySemanticModel",
		"query_dashboard_visual": "queryDashboardVisualData",
	} {
		operation, ok := operationsByName[name]
		if !ok {
			t.Fatalf("APIGenOperations() missing generated tool %q", name)
		}
		if operation.Tool.OperationID != operationID {
			t.Fatalf("tool %q operation = %q, want %q", name, operation.Tool.OperationID, operationID)
		}
		if operation.Tool.Effect != agenttool.EffectRead {
			t.Fatalf("tool %q effect = %q, want read", name, operation.Tool.Effect)
		}
	}
	if slices.Contains(APIGenToolNames(), "query_dashboard_page") {
		t.Fatalf("APIGenToolNames() = %#v, must not contain query_dashboard_page", APIGenToolNames())
	}
}

func TestToolNamesAreTheCuratedSurface(t *testing.T) {
	want := []string{
		"catalog_get",
		"catalog_list",
		"catalog_search",
		"docs_read",
		"docs_search",
		"query_dashboard_visual",
		"query_semantic_model",
		"query_visual",
	}
	if got := ToolNames(); !slices.Equal(got, want) {
		t.Fatalf("ToolNames() = %#v, want %#v", got, want)
	}
}
