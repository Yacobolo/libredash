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
	for _, operation := range operations {
		if operation.Tool.Effect != agenttool.EffectRead {
			t.Fatalf("tool %q effect = %q, want read", operation.Tool.Name, operation.Tool.Effect)
		}
		if operation.Tool.OperationID != operation.Contract.OperationID {
			t.Fatalf("tool %q operation = %q, registry operation = %q", operation.Tool.Name, operation.Tool.OperationID, operation.Contract.OperationID)
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
