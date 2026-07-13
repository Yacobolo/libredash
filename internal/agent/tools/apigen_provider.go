package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/libredash/internal/dataquery"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
	"github.com/Yacobolo/toolbelt/apigen/runtime/agenttool"
	"github.com/go-chi/chi/v5"
)

type Scope struct {
	WorkspaceID   string
	PrincipalID   string
	DevAuthBypass bool
	Credential    CredentialScope
}

type CredentialScope struct {
	WorkspaceID string
	Restricted  bool
	Privileges  []string
}

type APIGenAuthorizeFunc func(ctx context.Context, scope Scope, operationID string) (agentcore.ToolResult, bool)

type APIGenDispatchFunc func(operationID string, request *http.Request) (*http.Response, bool)

type APIGenProvider struct {
	Authorize APIGenAuthorizeFunc
	Dispatch  APIGenDispatchFunc
}

func (p APIGenProvider) Definitions(scope Scope) []agentcore.ToolDefinition {
	operations := APIGenOperations()
	definitions := make([]agentcore.ToolDefinition, 0, len(operations))
	for _, operation := range operations {
		operation := operation
		definitions = append(definitions, agentcore.ToolDefinition{
			Name:        operation.Tool.Name,
			Description: operation.Tool.Description,
			InputSchema: append(json.RawMessage(nil), operation.Tool.InputSchema...),
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				return p.Run(ctx, scope, operation, call), nil
			}),
		})
	}
	return definitions
}

func (p APIGenProvider) Run(ctx context.Context, scope Scope, operation APIGenOperation, call agentcore.ToolCall) agentcore.ToolResult {
	if p.Authorize == nil {
		return apigenAgentToolError("authorization_failed", "agent tool authorizer is not configured")
	}
	if errResult, ok := p.Authorize(ctx, scope, operation.Contract.OperationID); !ok {
		return errResult
	}
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{
		WorkspaceID: scope.WorkspaceID,
		Surface:     dataquery.SurfaceAgent,
		Operation:   dataquery.OperationAgentQuery,
		PrincipalID: scope.PrincipalID,
		RequestID:   call.ID,
		ObjectType:  "agent_tool",
		ObjectID:    operation.Tool.Name,
	})
	request, err := agenttool.BuildRequest(operation.Tool, call.Arguments, agenttool.Context{"workspace": scope.WorkspaceID})
	if err != nil {
		return agentToolRuntimeError(err)
	}
	request = request.WithContext(ctx)
	request.Header.Set("Accept", "application/json")
	request = withAPIGenRouteContext(request, operation.Tool.Path)
	if p.Dispatch == nil {
		return apigenAgentToolError("operation_not_found", "APIGen operation dispatcher is not configured")
	}
	response, ok := p.Dispatch(operation.Contract.OperationID, request)
	if !ok {
		return apigenAgentToolError("operation_not_found", "APIGen operation is not dispatchable")
	}
	result, err := agenttool.ProjectResponse(operation.Tool, response)
	if err != nil {
		return agentToolRuntimeError(err)
	}
	return agentcore.ToolResult{Content: result.Content, IsError: result.IsError}
}

func withAPIGenRouteContext(request *http.Request, pathTemplate string) *http.Request {
	templateSegments := strings.Split(strings.Trim(pathTemplate, "/"), "/")
	requestSegments := strings.Split(strings.Trim(request.URL.EscapedPath(), "/"), "/")
	if len(templateSegments) != len(requestSegments) {
		return request
	}
	routeContext := chi.NewRouteContext()
	for index, segment := range templateSegments {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
		value, err := url.PathUnescape(requestSegments[index])
		if err == nil {
			routeContext.URLParams.Add(name, value)
		}
	}
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func agentToolRuntimeError(err error) agentcore.ToolResult {
	if runtimeErr, ok := err.(*agenttool.Error); ok {
		return apigenAgentToolError(runtimeErr.Code, runtimeErr.Message)
	}
	return apigenAgentToolError("agent_tool_failed", err.Error())
}

func apigenAgentToolError(code, message string) agentcore.ToolResult {
	return agentcore.ToolResult{
		IsError: true,
		Content: map[string]any{
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		},
	}
}

func ToolError(code, message string) agentcore.ToolResult {
	return apigenAgentToolError(code, message)
}
