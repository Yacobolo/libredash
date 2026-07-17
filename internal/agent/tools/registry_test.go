package tools

import (
	"slices"
	"testing"

	"github.com/Yacobolo/toolbelt/apigen/runtime/agenttool"
)

func TestAPIGenOperationsUseGeneratedReadOnlyToolContracts(t *testing.T) {
	operations := APIGenOperations()
	if len(operations) != 26 {
		t.Fatalf("APIGenOperations() count = %d, want 26", len(operations))
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
		"list_semantic_model_fields":   "listSemanticModelFields",
		"query_semantic_model":         "querySemanticModel",
		"explain_semantic_model_query": "explainSemanticModelQuery",
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
	if !slices.Contains(APIGenToolNames(), "query_dashboard_page") {
		t.Fatalf("APIGenToolNames() = %#v, want query_dashboard_page", APIGenToolNames())
	}
}

func TestWorkspaceBindingIsTrustedContext(t *testing.T) {
	for _, operation := range APIGenOperations() {
		if operation.Tool.Name != "list_dashboards" {
			continue
		}
		for _, binding := range operation.Tool.Bindings {
			if binding.WireName == "workspace" {
				if binding.Mode != "context" || binding.ContextKey != "workspace" {
					t.Fatalf("workspace binding = %#v", binding)
				}
				return
			}
		}
		t.Fatal("list_dashboards has no workspace binding")
	}
	t.Fatal("list_dashboards tool not found")
}
