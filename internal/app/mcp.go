package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/Yacobolo/leapview/internal/access"
	agentcap "github.com/Yacobolo/leapview/internal/agent"
	agenttools "github.com/Yacobolo/leapview/internal/agent/tools"
	"github.com/Yacobolo/leapview/internal/brand"
	"github.com/Yacobolo/leapview/internal/staticasset"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *Server) mcpHandler() http.Handler {
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		server, err := s.mcpServer(r)
		if err != nil {
			s.logger.ErrorContext(r.Context(), "build MCP tool catalog failed", "error", err)
			return nil
		}
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       s.logger,
	})
	protected := s.protectMCPAccessToken(transport)
	originProtection := http.NewCrossOriginProtection()
	return originProtection.Handler(protected)
}

func (s *Server) mcpServer(r *http.Request) (*mcp.Server, error) {
	scope, ok := s.agentScopeForRequest(r)
	if !ok {
		return nil, fmt.Errorf("MCP request is missing its authenticated principal")
	}
	definitions := s.agentToolDefinitions(scope)
	sort.SliceStable(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	catalog, err := agentcore.NewToolCatalog(definitions)
	if err != nil {
		return nil, err
	}
	version := staticasset.Version()
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "leapview",
		Title:   brand.Name,
		Version: version,
	}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}})
	for _, definition := range catalog.Definitions() {
		definition := definition
		annotations := agenttools.AnnotationsForEffect(definition.Effect)
		server.AddTool(&mcp.Tool{
			Name:         definition.Name,
			Description:  definition.Description,
			InputSchema:  definition.InputSchema,
			OutputSchema: definition.OutputSchema,
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    annotations.ReadOnlyHint,
				DestructiveHint: &annotations.DestructiveHint,
				IdempotentHint:  annotations.IdempotentHint,
				OpenWorldHint:   &annotations.OpenWorldHint,
			},
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			arguments := request.Params.Arguments
			if len(arguments) == 0 {
				arguments = json.RawMessage(`{}`)
			}
			result, err := catalog.Execute(ctx, agentcore.ToolCall{
				ID:        fmt.Sprintf("mcp_%d", time.Now().UnixNano()),
				Name:      definition.Name,
				Arguments: arguments,
			})
			if err != nil {
				return mcpErrorResult("tool_execution_failed", err.Error()), nil
			}
			return mcpResult(result)
		})
	}
	return server, nil
}

func mcpResult(result agentcore.ToolResult) (*mcp.CallToolResult, error) {
	if result.Content == nil {
		return mcpErrorResult("tool_result_invalid", "tool returned no structured content"), nil
	}
	encoded, err := json.Marshal(result.Content)
	if err != nil {
		return mcpErrorResult("tool_result_invalid", "tool output was not JSON serializable"), nil
	}
	var structured map[string]any
	if err := json.Unmarshal(encoded, &structured); err != nil || structured == nil {
		return mcpErrorResult("tool_result_invalid", "tool output must be a JSON object"), nil
	}
	if result.IsError {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
			IsError: true,
		}, nil
	}
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
		StructuredContent: structured,
		IsError:           result.IsError,
	}, nil
}

func mcpErrorResult(code, message string) *mcp.CallToolResult {
	structured := map[string]any{"error": map[string]any{"code": code, "message": message}}
	encoded, _ := json.Marshal(structured)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
		IsError: true,
	}
}

func (s *Server) protectMCPAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || s.mcpOAuthResource == nil {
			if s.mcpOAuthResource != nil {
				s.mcpOAuthResource.Challenge(w)
			} else {
				writeBearerChallenge(w, r)
			}
			return
		}
		r = r.WithContext(access.WithAuthorizationCache(r.Context()))
		var principal Principal
		if s.auth.devBypass && s.auth.acceptsPublicBearer(r) {
			principal = localDeveloperPrincipal()
		} else {
			credential, err := s.mcpOAuthResource.Authenticate(r.Context(), bearerToken(r))
			if err != nil {
				s.mcpOAuthResource.Challenge(w)
				return
			}
			principal = Principal{
				ID: credential.Principal.ID, Email: credential.Principal.Email,
				DisplayName: credential.Principal.DisplayName,
			}
		}
		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		r = r.WithContext(ctx)
		if !principal.DevBypass {
			allowed, err := s.authorizeGlobalAgentPrivilege(r.Context(), principal.ID, nil, access.PrivilegeUseAgent)
			if err != nil {
				writeAuthError(w, r, err, http.StatusInternalServerError)
				return
			}
			if !allowed {
				writeAuthError(w, r, errForbidden, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) agentScopeForRequest(r *http.Request) (agentcap.Scope, bool) {
	if s.auth == nil {
		return agentcap.Scope{}, false
	}
	principal, ok := s.auth.Principal(r)
	if !ok {
		return agentcap.Scope{}, false
	}
	scope := agentcap.Scope{PrincipalID: principal.ID, DevAuthBypass: principal.DevBypass}
	if credential, ok := s.auth.APICredential(r); ok {
		scope.Credential.WorkspaceID = credential.Token.WorkspaceID
		scope.Credential.Restricted = credential.Token.Privileges != nil
		for _, privilege := range credential.Token.Privileges {
			scope.Credential.Privileges = append(scope.Credential.Privileges, string(privilege))
		}
	}
	return scope, true
}
