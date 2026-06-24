package cli

import (
	"fmt"
	"net/url"
	"strings"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	apigencli "github.com/Yacobolo/libredash/internal/cli/gen"
)

func apiOperationURL(target, operationID string, pathParams map[string]string, query url.Values) (string, error) {
	path, ok := deploymentV1CLIPath(operationID)
	if !ok {
		path, ok = generatedCLIPath(operationID)
	}
	if !ok {
		contract, contractOK := apigenapi.GetAPIGenOperationContract(operationID)
		if !contractOK {
			return "", fmt.Errorf("unknown API operation %q", operationID)
		}
		path = contract.Path
	}
	for name, value := range pathParams {
		path = strings.ReplaceAll(path, "{"+name+"}", url.PathEscape(value))
	}
	if strings.Contains(path, "{") {
		return "", fmt.Errorf("unresolved API path parameter in %q", path)
	}
	u, err := url.Parse(strings.TrimRight(target, "/") + path)
	if err != nil {
		return "", err
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func deploymentV1CLIPath(operationID string) (string, bool) {
	switch operationID {
	case "createDeployment", "listDeployments":
		return "/api/v1/workspaces/{workspace}/deployments", true
	case "getDeployment":
		return "/api/v1/workspaces/{workspace}/deployments/{deployment}", true
	case "uploadDeploymentArtifact":
		return "/api/v1/workspaces/{workspace}/deployments/{deployment}/artifact", true
	case "validateDeployment":
		return "/api/v1/workspaces/{workspace}/deployments/{deployment}/validate", true
	case "activateDeployment", "rollbackDeployment":
		return "/api/v1/workspaces/{workspace}/deployments/{deployment}/activate", true
	default:
		return "", false
	}
}

func generatedCLIPath(operationID string) (string, bool) {
	for _, spec := range apigencli.APIGeneratedCommandSpecs {
		if spec.OperationID == operationID {
			return spec.Path, true
		}
	}
	return "", false
}
