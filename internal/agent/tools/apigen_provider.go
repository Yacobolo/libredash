package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/dataquery"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
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

type APIGenDispatchFunc func(scope Scope, operationID string, request *http.Request) (*http.Response, bool)

type APIGenProvider struct {
	Authorize APIGenAuthorizeFunc
	Dispatch  APIGenDispatchFunc
}

func (p APIGenProvider) Definitions(scope Scope) []agentcore.ToolDefinition {
	operations := APIGenOperations()
	definitions := make([]agentcore.ToolDefinition, 0, len(operations))
	for _, operation := range operations {
		operation := operationForScope(operation, scope)
		definitions = append(definitions, agentcore.ToolDefinition{
			Name:         operation.Tool.Name,
			Description:  operation.Tool.Description,
			InputSchema:  append(json.RawMessage(nil), operation.Tool.InputSchema...),
			OutputSchema: requireToolObjectSchema(operation.Tool.OutputSchema),
			Effect:       string(operation.Tool.Effect),
			Tags:         append([]string(nil), operation.Tool.Tags...),
			Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
				return p.Run(ctx, scope, operation, call), nil
			}),
		})
	}
	return definitions
}

func requireToolObjectSchema(input json.RawMessage) json.RawMessage {
	var schema map[string]any
	if err := json.Unmarshal(input, &schema); err != nil || schema == nil {
		return append(json.RawMessage(nil), input...)
	}
	if _, ok := schema["type"]; ok {
		return append(json.RawMessage(nil), input...)
	}
	schema["type"] = "object"
	output, err := json.Marshal(schema)
	if err != nil {
		return append(json.RawMessage(nil), input...)
	}
	return output
}

func (p APIGenProvider) Run(ctx context.Context, scope Scope, operation APIGenOperation, call agentcore.ToolCall) agentcore.ToolResult {
	if p.Authorize == nil {
		return apigenAgentToolError("authorization_failed", "agent tool authorizer is not configured")
	}
	request, err := agenttool.BuildRequest(operation.Tool, call.Arguments, agenttool.Context{"workspace": scope.WorkspaceID})
	if err != nil {
		return agentToolRuntimeError(err)
	}
	request = withAPIGenRouteContext(request, operation.Tool.Path)
	runScope := scope
	if runScope.WorkspaceID == "" {
		runScope.WorkspaceID = strings.TrimSpace(chi.URLParam(request, "workspace"))
	}
	if errResult, ok := p.Authorize(ctx, runScope, operation.Contract.OperationID); !ok {
		return errResult
	}
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{
		WorkspaceID: runScope.WorkspaceID,
		Surface:     dataquery.SurfaceAgent,
		Operation:   dataquery.OperationAgentQuery,
		PrincipalID: runScope.PrincipalID,
		RequestID:   call.ID,
		ObjectType:  "agent_tool",
		ObjectID:    operation.Tool.Name,
	})
	request = request.WithContext(ctx)
	request = withAPIGenRouteContext(request, operation.Tool.Path)
	if p.Dispatch == nil {
		return apigenAgentToolError("operation_not_found", "APIGen operation dispatcher is not configured")
	}
	response, ok := p.Dispatch(runScope, operation.Contract.OperationID, request)
	if !ok {
		return apigenAgentToolError("operation_not_found", "APIGen operation is not dispatchable")
	}
	result, err := agenttool.ProjectResponse(operation.Tool, response)
	if err != nil {
		return agentToolRuntimeError(err)
	}
	return agentcore.ToolResult{Content: result.Content, IsError: result.IsError}
}

func operationForScope(operation APIGenOperation, scope Scope) APIGenOperation {
	if strings.TrimSpace(scope.WorkspaceID) != "" {
		return operation
	}
	tool := agenttool.CloneContract(operation.Tool)
	promoted := false
	for index := range tool.Bindings {
		binding := &tool.Bindings[index]
		if binding.Mode != "context" || binding.ContextKey != "workspace" {
			continue
		}
		binding.Argument = "workspace"
		binding.Mode = "model"
		binding.ContextKey = ""
		binding.Required = true
		promoted = true
	}
	if promoted {
		tool.InputSchema = requireToolStringProperty(tool.InputSchema, "workspace")
	}
	operation.Tool = tool
	return operation
}

func requireToolStringProperty(input json.RawMessage, name string) json.RawMessage {
	var schema map[string]any
	if err := json.Unmarshal(input, &schema); err != nil {
		return input
	}
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	properties[name] = map[string]any{
		"type":        "string",
		"minLength":   1,
		"description": "Workspace ID to query.",
	}
	required, _ := schema["required"].([]any)
	for _, item := range required {
		if item == name {
			encoded, err := json.Marshal(schema)
			if err == nil {
				return encoded
			}
			return input
		}
	}
	schema["required"] = append(required, name)
	encoded, err := json.Marshal(schema)
	if err != nil {
		return input
	}
	return encoded
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
