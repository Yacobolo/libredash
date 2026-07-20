package tools

import (
	"sort"

	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	agenttool "github.com/Yacobolo/toolbelt/apigen/runtime/agenttool"
)

const QueryVisualToolName = "query_visual"

type APIGenOperation struct {
	Contract apigenapi.GenOperationContract
	Tool     agenttool.Contract
}

func APIGenOperations() []APIGenOperation {
	operationContracts := apigenapi.GetAPIGenOperationContracts()
	toolContracts := apigenapi.GetAPIGenToolContracts()
	operations := make([]APIGenOperation, 0, len(toolContracts))
	for _, tool := range toolContracts {
		contract, ok := operationContracts[tool.OperationID]
		if !ok || !operationAllowed(contract, tool) {
			continue
		}
		operations = append(operations, APIGenOperation{Contract: contract, Tool: tool})
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Tool.Name < operations[j].Tool.Name
	})
	return operations
}

func APIGenToolNames() []string {
	operations := APIGenOperations()
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Tool.Name)
	}
	return names
}

func ManualToolNames() []string {
	return []string{QueryVisualToolName}
}

func ToolNames() []string {
	names := append([]string{}, APIGenToolNames()...)
	names = append(names, ManualToolNames()...)
	sort.Strings(names)
	return names
}

func IsKnownTool(name string) bool {
	for _, tool := range ToolNames() {
		if tool == name {
			return true
		}
	}
	return false
}

func operationAllowed(contract apigenapi.GenOperationContract, tool agenttool.Contract) bool {
	if tool.Effect != agenttool.EffectRead || contract.Manual {
		return false
	}
	if contract.Method != "GET" && contract.Method != "POST" {
		return false
	}
	switch operationPrivilege(contract) {
	case "USE_WORKSPACE", "VIEW_ITEM", "QUERY_DATA", "PREVIEW_DATA", "REFRESH_DATA":
		return true
	default:
		return false
	}
}

func operationPrivilege(contract apigenapi.GenOperationContract) string {
	raw, _ := contract.Extensions["x-authz"].(map[string]any)
	value, _ := raw["privilege"].(string)
	return value
}
