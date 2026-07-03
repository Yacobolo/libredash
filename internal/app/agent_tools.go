package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/agenttools"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/Yacobolo/libredash/pkg/agent"
	"github.com/go-chi/chi/v5"
)

type apigenAgentParameter struct {
	Name     string
	In       string
	Required bool
	Schema   map[string]any
}

type apigenAgentOperation struct {
	Contract           apigenapi.GenOperationContract
	Extension          agenttools.Extension
	Parameters         []apigenAgentParameter
	BodyProperties     map[string]any
	BodyRequiredFields []string
	Summary            string
}

func (s *Server) configureAgentTools() {
	if s.agent == nil {
		return
	}
	if s.store != nil {
		s.agent.SetSystemPromptProvider(s.agentSystemPrompt)
	}
	s.agent.SetPolicyProvider(s.agentPolicyForScope)
	s.agent.AppendToolProviders(s.agentVisualToolDefinitions, s.agentAPIGenToolDefinitions)
}

func (s *Server) agentPolicyForScope(scope agentapp.Scope) (workspace.AgentPolicy, bool) {
	metrics, ok := s.metricsForWorkspace(scope.WorkspaceID)
	if !ok || metrics == nil {
		return workspace.AgentPolicy{}, false
	}
	provider, ok := metrics.(agentPolicyProvider)
	if !ok {
		return workspace.AgentPolicy{}, false
	}
	return provider.AgentPolicy(), true
}

func (s *Server) agentAPIGenToolDefinitions(scope agentapp.Scope) []agent.ToolDefinition {
	operations := apigenAgentOperations()
	tools := make([]agent.ToolDefinition, 0, len(operations))
	for _, operation := range operations {
		operation := operation
		tools = append(tools, agent.ToolDefinition{
			Name:        operation.Extension.Name,
			Description: apigenAgentToolDescription(operation),
			InputSchema: apigenAgentInputSchema(operation),
			Handler: agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
				return s.runAPIGenAgentTool(ctx, scope, operation, call), nil
			}),
		})
	}
	return tools
}

func apigenAgentOperations() []apigenAgentOperation {
	spec, err := apigenapi.GetEmbeddedOpenAPISpec()
	if err != nil {
		return nil
	}
	paths, _ := spec["paths"].(map[string]any)
	registry := agenttools.APIGenOperations()
	operations := make([]apigenAgentOperation, 0, len(registry))
	for _, entry := range registry {
		openapiOperation, ok := openAPIOperation(paths, entry.Contract)
		if !ok {
			continue
		}
		operations = append(operations, apigenAgentOperation{
			Contract:           entry.Contract,
			Extension:          entry.Extension,
			Parameters:         apigenAgentParameters(openapiOperation),
			BodyProperties:     apigenAgentBodyProperties(spec, openapiOperation),
			BodyRequiredFields: apigenAgentBodyRequiredFields(spec, openapiOperation),
			Summary:            stringFromMap(openapiOperation, "summary"),
		})
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Extension.Name < operations[j].Extension.Name
	})
	return operations
}

func openAPIOperation(paths map[string]any, contract apigenapi.GenOperationContract) (map[string]any, bool) {
	pathItem, ok := paths[contract.Path].(map[string]any)
	if !ok {
		return nil, false
	}
	operation, ok := pathItem[strings.ToLower(contract.Method)].(map[string]any)
	return operation, ok
}

func apigenAgentParameters(operation map[string]any) []apigenAgentParameter {
	rawParams, _ := operation["parameters"].([]any)
	parameters := make([]apigenAgentParameter, 0, len(rawParams))
	for _, raw := range rawParams {
		param, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		schema, _ := param["schema"].(map[string]any)
		parameters = append(parameters, apigenAgentParameter{
			Name:     stringFromMap(param, "name"),
			In:       stringFromMap(param, "in"),
			Required: boolFromMap(param, "required"),
			Schema:   portableAgentToolSchema(cloneStringAnyMap(schema)),
		})
	}
	return parameters
}

func apigenAgentBodyProperties(spec map[string]any, operation map[string]any) map[string]any {
	schema := apigenAgentRequestBodySchema(spec, operation)
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	out := make(map[string]any, len(properties))
	for name, value := range properties {
		if property, ok := value.(map[string]any); ok {
			out[name] = portableAgentToolSchema(inlineOpenAPISchemaRefs(spec, property, map[string]bool{}))
		}
	}
	return out
}

func apigenAgentBodyRequiredFields(spec map[string]any, operation map[string]any) []string {
	schema := apigenAgentRequestBodySchema(spec, operation)
	rawRequired, _ := schema["required"].([]any)
	required := make([]string, 0, len(rawRequired))
	for _, raw := range rawRequired {
		if value, ok := raw.(string); ok && value != "" {
			required = append(required, value)
		}
	}
	return required
}

func apigenAgentRequestBodySchema(spec map[string]any, operation map[string]any) map[string]any {
	requestBody, _ := operation["requestBody"].(map[string]any)
	content, _ := requestBody["content"].(map[string]any)
	jsonContent, _ := content["application/json"].(map[string]any)
	schema, _ := jsonContent["schema"].(map[string]any)
	return resolveOpenAPISchemaRef(spec, schema)
}

func resolveOpenAPISchemaRef(spec map[string]any, schema map[string]any) map[string]any {
	ref := stringFromMap(schema, "$ref")
	if ref == "" || !strings.HasPrefix(ref, "#/components/schemas/") {
		return schema
	}
	name := strings.TrimPrefix(ref, "#/components/schemas/")
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	resolved, _ := schemas[name].(map[string]any)
	return resolved
}

func inlineOpenAPISchemaRefs(spec map[string]any, schema map[string]any, seen map[string]bool) map[string]any {
	ref := stringFromMap(schema, "$ref")
	if ref != "" {
		if seen[ref] {
			return map[string]any{"type": "object"}
		}
		seen[ref] = true
		return inlineOpenAPISchemaRefs(spec, resolveOpenAPISchemaRef(spec, schema), seen)
	}
	out := cloneStringAnyMap(schema)
	for key, value := range out {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = inlineOpenAPISchemaRefs(spec, typed, seen)
		case []any:
			items := make([]any, len(typed))
			for i, item := range typed {
				if itemMap, ok := item.(map[string]any); ok {
					items[i] = inlineOpenAPISchemaRefs(spec, itemMap, seen)
				} else {
					items[i] = item
				}
			}
			out[key] = items
		}
	}
	return out
}

func portableAgentToolSchema(schema map[string]any) map[string]any {
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		if !portableAgentToolSchemaKeys[key] {
			continue
		}
		switch key {
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				continue
			}
			cleanProperties := make(map[string]any, len(properties))
			for name, rawProperty := range properties {
				if property, ok := rawProperty.(map[string]any); ok {
					cleanProperties[name] = portableAgentToolSchema(property)
				}
			}
			out[key] = cleanProperties
		case "items":
			if items, ok := value.(map[string]any); ok {
				out[key] = portableAgentToolSchema(items)
			}
		case "additionalProperties":
			if nested, ok := value.(map[string]any); ok {
				out[key] = portableAgentToolSchema(nested)
			} else {
				out[key] = value
			}
		default:
			out[key] = value
		}
	}
	return out
}

var portableAgentToolSchemaKeys = map[string]bool{
	"additionalProperties": true,
	"description":          true,
	"enum":                 true,
	"items":                true,
	"maximum":              true,
	"maxLength":            true,
	"minimum":              true,
	"minLength":            true,
	"properties":           true,
	"required":             true,
	"type":                 true,
}

func apigenAgentToolDescription(operation apigenAgentOperation) string {
	if operation.Summary != "" {
		return operation.Summary + "."
	}
	return "Call the LibreDash " + operation.Contract.OperationID + " API operation."
}

func apigenAgentInputSchema(operation apigenAgentOperation) json.RawMessage {
	properties := map[string]any{}
	required := []string{}
	for _, parameter := range operation.Parameters {
		if parameter.Name == "" {
			continue
		}
		if parameter.Name == "workspace" {
			parameter.Required = false
		}
		properties[parameter.Name] = parameter.Schema
		if parameter.Required {
			required = append(required, parameter.Name)
		}
	}
	for name, schema := range operation.BodyProperties {
		properties[name] = schema
		if agentStringSliceHas(operation.BodyRequiredFields, name) {
			required = append(required, name)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	out, err := json.Marshal(schema)
	if err != nil {
		return json.RawMessage(`{"type":"object","additionalProperties":false}`)
	}
	return out
}

func agentStringSliceHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (s *Server) runAPIGenAgentTool(ctx context.Context, scope agentapp.Scope, operation apigenAgentOperation, call agent.ToolCall) agent.ToolResult {
	args, err := decodeAPIGenAgentToolArguments(call.Arguments)
	if err != nil {
		return apigenAgentToolError("invalid_arguments", err.Error())
	}
	toolScope, err := apigenAgentToolScope(scope, operation, args)
	if err != nil {
		return apigenAgentToolError("invalid_arguments", err.Error())
	}
	if errResult, ok := s.authorizeAPIGenAgentTool(ctx, toolScope, operation); !ok {
		return errResult
	}
	ctx = dataquery.WithMetadata(ctx, dataquery.Metadata{
		WorkspaceID: toolScope.WorkspaceID,
		Surface:     dataquery.SurfaceAgent,
		Operation:   dataquery.OperationAgentQuery,
		PrincipalID: toolScope.PrincipalID,
		RequestID:   call.ID,
		ObjectType:  "agent_tool",
		ObjectID:    operation.Extension.Name,
	})
	request, err := apigenAgentToolRequest(ctx, toolScope, operation, args)
	if err != nil {
		return apigenAgentToolError("invalid_arguments", err.Error())
	}
	recorder := httptest.NewRecorder()
	if ok := apigenapi.DispatchAPIGenOperation(operation.Contract.OperationID, apiGenAdapter{server: s}, recorder, request); !ok {
		return apigenAgentToolError("operation_not_found", "APIGen operation is not dispatchable")
	}
	return apigenAgentToolResult(operation.Extension, recorder.Result())
}

func (s *Server) authorizeAPIGenAgentTool(ctx context.Context, scope agentapp.Scope, operation apigenAgentOperation) (agent.ToolResult, bool) {
	permission := apigenOperationPermissions[operation.Contract.OperationID]
	if permission == "" {
		return apigenAgentToolError("forbidden", "operation has no LibreDash permission mapping"), false
	}
	return s.authorizeAgentPermission(ctx, scope, permission)
}

func (s *Server) authorizeAgentPermission(ctx context.Context, scope agentapp.Scope, permission string) (agent.ToolResult, bool) {
	if scope.PrincipalID == "" {
		return apigenAgentToolError("unauthorized", "agent tool requires an authenticated principal"), false
	}
	if !agentCredentialAllows(scope, permission) {
		return apigenAgentToolError("forbidden", "credential is not allowed to call this tool"), false
	}
	if scope.DevAuthBypass {
		return agent.ToolResult{}, true
	}
	repo, err := s.accessRepository()
	if err != nil {
		return apigenAgentToolError("authorization_failed", err.Error()), false
	}
	if repo == nil {
		return agent.ToolResult{}, true
	}
	allowed, err := repo.HasPermission(ctx, scope.WorkspaceID, scope.PrincipalID, permission)
	if err != nil {
		return apigenAgentToolError("authorization_failed", err.Error()), false
	}
	if !allowed {
		return apigenAgentToolError("forbidden", "principal does not have permission to call this tool"), false
	}
	return agent.ToolResult{}, true
}

func agentCredentialAllows(scope agentapp.Scope, permission string) bool {
	credential := scope.Credential
	if credential.WorkspaceID != "" && credential.WorkspaceID != scope.WorkspaceID {
		return false
	}
	if !credential.Restricted {
		return true
	}
	for _, allowed := range credential.Permissions {
		if allowed == permission {
			return true
		}
	}
	return false
}

func decodeAPIGenAgentToolArguments(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil {
		return nil, err
	}
	return args, nil
}

func apigenAgentToolRequest(ctx context.Context, scope agentapp.Scope, operation apigenAgentOperation, args map[string]any) (*http.Request, error) {
	path := operation.Contract.Path
	routeContext := chi.NewRouteContext()
	query := url.Values{}
	body, err := apigenAgentRequestBody(operation, args)
	if err != nil {
		return nil, err
	}
	for _, parameter := range operation.Parameters {
		switch parameter.In {
		case "path":
			value, err := apigenAgentPathValue(scope, parameter, args)
			if err != nil {
				return nil, err
			}
			path = strings.ReplaceAll(path, "{"+parameter.Name+"}", url.PathEscape(value))
			routeContext.URLParams.Add(parameter.Name, value)
		case "query":
			value, ok, err := apigenAgentStringArgument(parameter.Name, args)
			if err != nil {
				return nil, err
			}
			if !ok && parameter.Name == "limit" && operation.Extension.DefaultLimit > 0 {
				value = strconv.Itoa(operation.Extension.DefaultLimit)
				ok = true
			}
			if ok {
				query.Set(parameter.Name, value)
			}
		}
	}
	u := &url.URL{Scheme: "http", Host: "libredash.agent.local", Path: path, RawQuery: query.Encode()}
	request, err := http.NewRequestWithContext(ctx, operation.Contract.Method, u.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext)), nil
}

func apigenAgentPathValue(scope agentapp.Scope, parameter apigenAgentParameter, args map[string]any) (string, error) {
	if parameter.Name == "workspace" {
		return scope.WorkspaceID, nil
	}
	value, ok, err := apigenAgentStringArgument(parameter.Name, args)
	if err != nil {
		return "", err
	}
	if parameter.Required && (!ok || value == "") {
		return "", fmt.Errorf("%s is required", parameter.Name)
	}
	return value, nil
}

func apigenAgentToolScope(scope agentapp.Scope, operation apigenAgentOperation, args map[string]any) (agentapp.Scope, error) {
	for _, parameter := range operation.Parameters {
		if parameter.In != "path" || parameter.Name != "workspace" {
			continue
		}
		workspaceID, ok, err := apigenAgentStringArgument("workspace", args)
		if err != nil {
			return agentapp.Scope{}, err
		}
		if ok && strings.TrimSpace(workspaceID) != "" {
			scope.WorkspaceID = strings.TrimSpace(workspaceID)
		}
		if strings.TrimSpace(scope.WorkspaceID) == "" {
			return agentapp.Scope{}, fmt.Errorf("workspace is required")
		}
		return scope, nil
	}
	return scope, nil
}

func apigenAgentRequestBody(operation apigenAgentOperation, args map[string]any) (io.Reader, error) {
	if operation.Contract.Method != http.MethodPost || len(operation.BodyProperties) == 0 {
		return nil, nil
	}
	body := map[string]any{}
	for name := range operation.BodyProperties {
		if value, ok := args[name]; ok {
			body[name] = value
		}
	}
	if _, ok := body["limit"]; !ok && operation.Extension.DefaultLimit > 0 {
		if _, hasLimit := operation.BodyProperties["limit"]; hasLimit {
			body["limit"] = operation.Extension.DefaultLimit
		}
	}
	if len(body) == 0 && !operation.Contract.RequestBodyRequired {
		return nil, nil
	}
	for _, name := range operation.BodyRequiredFields {
		if _, ok := body[name]; !ok {
			return nil, fmt.Errorf("%s is required", name)
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(encoded), nil
}

func apigenAgentStringArgument(name string, args map[string]any) (string, bool, error) {
	value, ok := args[name]
	if !ok || value == nil {
		return "", false, nil
	}
	switch v := value.(type) {
	case string:
		return v, true, nil
	case json.Number:
		return v.String(), true, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true, nil
	case bool:
		return strconv.FormatBool(v), true, nil
	default:
		return "", false, fmt.Errorf("%s must be a scalar value", name)
	}
}

func apigenAgentToolResult(extension agenttools.Extension, response *http.Response) agent.ToolResult {
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	content := map[string]any{
		"status": response.StatusCode,
	}
	if len(bytes.TrimSpace(body)) > 0 {
		var decoded any
		if err := json.Unmarshal(body, &decoded); err == nil {
			content["body"] = decoded
		} else {
			content["body"] = string(body)
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return agent.ToolResult{Content: content, IsError: true}
	}
	if body, ok := content["body"]; ok {
		return agent.ToolResult{Content: shapeAPIGenAgentToolContent(body, extension.Output)}
	}
	return agent.ToolResult{Content: content}
}

func shapeAPIGenAgentToolContent(content any, output agenttools.Output) any {
	if apigenAgentOutputEmpty(output) {
		return content
	}
	shaped := map[string]any{}
	for _, field := range output.RootFields {
		if value, ok := valueAtPath(content, field); ok {
			shaped[agentOutputFieldName(field)] = value
		}
	}
	if output.ItemsPath != "" {
		applyAPIGenAgentCollection(shaped, content, agenttools.OutputCollection{
			Path:   output.ItemsPath,
			As:     "items",
			Fields: output.Fields,
			Count:  output.Count,
		})
	}
	for _, collection := range output.Collections {
		applyAPIGenAgentCollection(shaped, content, collection)
	}
	for _, outputMap := range output.Maps {
		applyAPIGenAgentMap(shaped, content, outputMap)
	}
	if output.CursorPath != "" {
		cursor := stringValueAtPath(content, output.CursorPath)
		if cursor != "" {
			shaped["nextCursor"] = cursor
			shaped["hasMore"] = true
		} else {
			shaped["hasMore"] = false
		}
	}
	if _, ok := shaped["hasMore"]; !ok {
		deriveAPIGenAgentHasMore(shaped)
	}
	return shaped
}

func apigenAgentOutputEmpty(output agenttools.Output) bool {
	return output.ItemsPath == "" &&
		len(output.Fields) == 0 &&
		output.CursorPath == "" &&
		!output.Count &&
		len(output.RootFields) == 0 &&
		len(output.Collections) == 0 &&
		len(output.Maps) == 0
}

func applyAPIGenAgentCollection(shaped map[string]any, content any, collection agenttools.OutputCollection) {
	if collection.Path == "" || collection.As == "" {
		return
	}
	value, ok := valueAtPath(content, collection.Path)
	if !ok {
		return
	}
	items, ok := value.([]any)
	if !ok {
		return
	}
	shaped[collection.As] = projectAPIGenAgentItems(items, collection.Fields)
	if collection.Count {
		shaped["count"] = len(items)
	}
}

func applyAPIGenAgentMap(shaped map[string]any, content any, outputMap agenttools.OutputMap) {
	if outputMap.Path == "" || outputMap.As == "" {
		return
	}
	value, ok := valueAtPath(content, outputMap.Path)
	if !ok {
		return
	}
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	out := make(map[string]any, len(object))
	for key, value := range object {
		projected := projectAPIGenAgentObject(value, outputMap.Fields)
		if outputMap.Collection.Path != "" && outputMap.Collection.As != "" {
			applyAPIGenAgentCollection(projected, value, outputMap.Collection)
		}
		out[key] = projected
	}
	shaped[outputMap.As] = out
}

func projectAPIGenAgentItems(items []any, fields []string) []any {
	if len(fields) == 0 {
		out := make([]any, len(items))
		copy(out, items)
		return out
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		projected := map[string]any{}
		for _, field := range fields {
			if value, ok := object[field]; ok {
				projected[field] = value
			}
		}
		out = append(out, projected)
	}
	return out
}

func projectAPIGenAgentObject(value any, fields []string) map[string]any {
	object, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	if len(fields) == 0 {
		out := make(map[string]any, len(object))
		for key, value := range object {
			out[key] = value
		}
		return out
	}
	projected := map[string]any{}
	for _, field := range fields {
		if value, ok := valueAtPath(object, field); ok {
			projected[agentOutputFieldName(field)] = value
		}
	}
	return projected
}

func deriveAPIGenAgentHasMore(shaped map[string]any) {
	count, ok := intFromAny(shaped["count"])
	if !ok {
		return
	}
	availableRows, ok := intFromAny(shaped["availableRows"])
	if !ok {
		return
	}
	shaped["hasMore"] = availableRows > count
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func agentOutputFieldName(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

func valueAtPath(value any, path string) (any, bool) {
	current := value
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func stringValueAtPath(value any, path string) string {
	raw, ok := valueAtPath(value, path)
	if !ok {
		return ""
	}
	text, _ := raw.(string)
	return strings.TrimSpace(text)
}

func apigenAgentToolError(code, message string) agent.ToolResult {
	return agent.ToolResult{
		IsError: true,
		Content: map[string]any{
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		},
	}
}

func stringFromMap(values map[string]any, key string) string {
	if value, ok := values[key].(string); ok {
		return value
	}
	return ""
}

func boolFromMap(values map[string]any, key string) bool {
	if value, ok := values[key].(bool); ok {
		return value
	}
	return false
}

func intFromMap(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(value.String())
		return parsed
	default:
		return 0
	}
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
