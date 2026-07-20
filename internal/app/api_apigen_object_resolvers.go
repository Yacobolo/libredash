package app

import (
	"strings"

	"github.com/Yacobolo/leapview/internal/access/httpauth"
	queryhttp "github.com/Yacobolo/leapview/internal/analytics/query/http"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	dashboardhttp "github.com/Yacobolo/leapview/internal/dashboard/http"
	workspacehttp "github.com/Yacobolo/leapview/internal/workspace/http"
)

const apiGenObjectScopeExtension = "x-leapview-object-scope"

type apiGenObjectScope struct {
	pathParameter string
	resolver      httpauth.ObjectResolver
}

// TypeSpec assigns operations to these domain scopes. The handwritten boundary
// only maps a stable scope name to the domain behavior that resolves objects.
var apiGenObjectScopes = map[string]apiGenObjectScope{
	"dashboard":       {pathParameter: "dashboard", resolver: dashboardhttp.DashboardObjectRefs},
	"semantic-model":  {pathParameter: "model", resolver: queryhttp.SemanticDatasetObjectRefs},
	"workspace-asset": {pathParameter: "assetId", resolver: workspacehttp.AssetObjectRefs},
}

func apigenOperationObjectResolver(operationID string) (httpauth.ObjectResolver, bool) {
	contract, ok := apigenapi.GetAPIGenOperationContract(operationID)
	if !ok {
		return nil, false
	}
	return apigenObjectResolverForContract(contract)
}

func apigenObjectResolverForContract(contract apigenapi.GenOperationContract) (httpauth.ObjectResolver, bool) {
	expectedScope, ambiguous := apigenObjectScopeForPath(contract.Path)
	if ambiguous {
		return nil, false
	}
	rawScope, hasScope := contract.Extensions[apiGenObjectScopeExtension]
	if !hasScope {
		return nil, expectedScope == ""
	}
	scope, ok := rawScope.(string)
	if !ok || scope == "" || scope != expectedScope {
		return nil, false
	}
	definition, ok := apiGenObjectScopes[scope]
	if !ok || definition.resolver == nil {
		return nil, false
	}
	return definition.resolver, true
}

func apigenObjectScopeForPath(path string) (string, bool) {
	matched := ""
	for scope, definition := range apiGenObjectScopes {
		if !strings.Contains(path, "{"+definition.pathParameter+"}") {
			continue
		}
		if matched != "" {
			return "", true
		}
		matched = scope
	}
	return matched, false
}
