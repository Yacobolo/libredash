package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	agentcap "github.com/Yacobolo/leapview/internal/agent"
	agenttools "github.com/Yacobolo/leapview/internal/agent/tools"
	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/Yacobolo/leapview/internal/dataquery"
	agentcore "github.com/Yacobolo/leapview/pkg/agent"
)

func (s *Server) configureAgentTools() {
	if s.agent != nil && s.store != nil {
		s.agent.SetSystemPromptProvider(s.agentSystemPrompt)
	}
	if s.agent == nil {
		return
	}
	s.agent.AppendToolProviders(
		func(scope agentcap.Scope) []agentcore.ToolDefinition {
			return s.agentToolDefinitions(scope)
		},
	)
}

// agentToolDefinitions is the single governed tool catalog consumed by the
// built-in agent and protocol adapters such as MCP.
func (s *Server) agentToolDefinitions(scope agentcap.Scope) []agentcore.ToolDefinition {
	toolScope := agentToolsScope(scope)
	definitions := s.agentVisualToolProvider().Definitions(toolScope)
	definitions = append(definitions, s.agentAPIGenToolProvider().Definitions(toolScope)...)
	return definitions
}

func (s *Server) agentVisualToolProvider() agenttools.VisualProvider {
	return agenttools.VisualProvider{
		Authorize: func(ctx context.Context, scope agenttools.Scope, request agenttools.VisualAuthorizationRequest) (agentcore.ToolResult, bool) {
			agentScope := agentScopeFromTools(scope)
			model := access.ItemObjectWithParent(access.SecurableSemanticModel, agentScope.WorkspaceID, request.Model, access.WorkspaceObject(agentScope.WorkspaceID))
			objects := []access.ObjectRef{
				access.ItemObjectWithParent(access.SecurableDataset, agentScope.WorkspaceID, request.Model+"/"+request.Dataset, model),
				model,
				access.WorkspaceObject(agentScope.WorkspaceID),
			}
			return s.authorizeAgentPrivilege(ctx, agentScope, access.PrivilegeQueryData, objects, "agent_tool", request.ToolName)
		},
		SemanticModel: func(workspaceID, modelID string) (model *semanticmodel.Model, ok bool) {
			metrics, ok := s.metricsForWorkspace(workspaceID)
			if !ok || metrics == nil {
				return nil, false
			}
			return metrics.SemanticModel(modelID)
		},
		AggregateRows: func(ctx context.Context, workspaceID, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
			metrics, ok := s.metricsForWorkspace(workspaceID)
			if !ok || metrics == nil {
				return nil, fmt.Errorf("unknown workspace %q", workspaceID)
			}
			return executeAggregateRows(ctx, metrics, modelID, request)
		},
		PreviewRows: func(ctx context.Context, workspaceID, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
			metrics, ok := s.metricsForWorkspace(workspaceID)
			if !ok || metrics == nil {
				return nil, fmt.Errorf("unknown workspace %q", workspaceID)
			}
			return executePreviewRows(ctx, metrics, modelID, request)
		},
		Histogram: func(ctx context.Context, workspaceID, modelID string, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error) {
			metrics, ok := s.metricsForWorkspace(workspaceID)
			if !ok || metrics == nil {
				return nil, fmt.Errorf("unknown workspace %q", workspaceID)
			}
			return executeHistogram(ctx, metrics, modelID, request, binCount)
		},
		Distribution: func(ctx context.Context, workspaceID, modelID string, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error) {
			metrics, ok := s.metricsForWorkspace(workspaceID)
			if !ok || metrics == nil {
				return nil, fmt.Errorf("unknown workspace %q", workspaceID)
			}
			return executeDistribution(ctx, metrics, modelID, request, sort, limit)
		},
	}
}

func (s *Server) agentAPIGenToolProvider() agenttools.APIGenProvider {
	return agenttools.APIGenProvider{
		Authorize: func(ctx context.Context, scope agenttools.Scope, operationID string) (agentcore.ToolResult, bool) {
			return s.authorizeAPIGenAgentOperation(ctx, agentScopeFromTools(scope), operationID)
		},
		Dispatch: func(scope agenttools.Scope, operationID string, request *http.Request) (*http.Response, bool) {
			principal := Principal{
				ID:        scope.PrincipalID,
				DevBypass: scope.DevAuthBypass,
			}
			if s.auth == nil {
				principal = localDeveloperPrincipal()
			}
			ctx := context.WithValue(request.Context(), principalContextKey{}, principal)
			if scope.Credential.Restricted || scope.Credential.WorkspaceID != "" || len(scope.Credential.Privileges) > 0 {
				privileges := make([]access.Privilege, 0, len(scope.Credential.Privileges))
				for _, privilege := range scope.Credential.Privileges {
					privileges = append(privileges, access.Privilege(privilege))
				}
				credential := access.APICredential{
					Principal: access.Principal{ID: scope.PrincipalID},
					Token: access.APIToken{
						ID:          "agent",
						PrincipalID: scope.PrincipalID,
						WorkspaceID: scope.Credential.WorkspaceID,
						Privileges:  privileges,
					},
				}
				ctx = context.WithValue(ctx, apiCredentialContextKey{}, credential)
			}
			request = request.WithContext(ctx)
			recorder := httptest.NewRecorder()
			if ok := apigenapi.DispatchAPIGenOperation(operationID, apiGenAdapter{server: s}, apiGenTransportErrorResponder{server: s}, recorder, request); !ok {
				return nil, false
			}
			return recorder.Result(), true
		},
	}
}

func agentToolsScope(scope agentcap.Scope) agenttools.Scope {
	return agenttools.Scope{
		WorkspaceID:   scope.WorkspaceID,
		PrincipalID:   scope.PrincipalID,
		DevAuthBypass: scope.DevAuthBypass,
		Credential: agenttools.CredentialScope{
			WorkspaceID: scope.Credential.WorkspaceID,
			Restricted:  scope.Credential.Restricted,
			Privileges:  append([]string{}, scope.Credential.Privileges...),
		},
	}
}

func agentScopeFromTools(scope agenttools.Scope) agentcap.Scope {
	return agentcap.Scope{
		WorkspaceID:   scope.WorkspaceID,
		PrincipalID:   scope.PrincipalID,
		DevAuthBypass: scope.DevAuthBypass,
		Credential: agentcap.CredentialScope{
			WorkspaceID: scope.Credential.WorkspaceID,
			Restricted:  scope.Credential.Restricted,
			Privileges:  append([]string{}, scope.Credential.Privileges...),
		},
	}
}

func (s *Server) authorizeAPIGenAgentOperation(ctx context.Context, scope agentcap.Scope, operationID string) (agentcore.ToolResult, bool) {
	privilege, ok := apigenOperationPrivilege(operationID)
	if !ok {
		return agenttools.ToolError("forbidden", "operation has no generated LeapView privilege metadata"), false
	}
	if operationID == "search" || (operationID == "listWorkspaces" && strings.TrimSpace(scope.WorkspaceID) == "") {
		if strings.TrimSpace(scope.PrincipalID) == "" {
			return agenttools.ToolError("unauthorized", "agent tool requires an authenticated principal"), false
		}
		if !agentCredentialAllowsPrivilege(scope, privilege) {
			return agenttools.ToolError("forbidden", "credential is not allowed to call this tool"), false
		}
		return agentcore.ToolResult{}, true
	}
	return s.authorizeAgentPrivilege(ctx, scope, privilege, []access.ObjectRef{access.WorkspaceObject(scope.WorkspaceID)}, "agent_tool", operationID)
}

func (s *Server) authorizeAgentPrivilege(ctx context.Context, scope agentcap.Scope, privilege access.Privilege, objects []access.ObjectRef, targetType, targetID string) (agentcore.ToolResult, bool) {
	if scope.PrincipalID == "" {
		return agenttools.ToolError("unauthorized", "agent tool requires an authenticated principal"), false
	}
	if !agentCredentialAllowsPrivilege(scope, privilege) {
		if repo, err := s.accessRepository(); err == nil && repo != nil {
			recordAgentToolAudit(ctx, repo, scope, privilege, targetType, targetID, "denied", fmt.Errorf("credential restriction"))
		}
		return agenttools.ToolError("forbidden", "credential is not allowed to call this tool"), false
	}
	if scope.DevAuthBypass {
		return agentcore.ToolResult{}, true
	}
	repo, err := s.accessRepository()
	if err != nil {
		return agenttools.ToolError("authorization_failed", err.Error()), false
	}
	if repo == nil {
		return agentcore.ToolResult{}, true
	}
	decision, err := repo.AuthorizeAny(ctx, scope.PrincipalID, privilege, objects)
	if err != nil {
		recordAgentToolAudit(ctx, repo, scope, privilege, targetType, targetID, "error", err)
		return agenttools.ToolError("authorization_failed", err.Error()), false
	}
	if !decision.Allowed {
		recordAgentToolAudit(ctx, repo, scope, privilege, targetType, targetID, "denied", nil)
		return agenttools.ToolError("forbidden", "principal does not have privilege to call this tool"), false
	}
	recordAgentToolAudit(ctx, repo, scope, privilege, targetType, targetID, "success", nil)
	return agentcore.ToolResult{}, true
}

func recordAgentToolAudit(ctx context.Context, repo access.Repository, scope agentcap.Scope, privilege access.Privilege, targetType, targetID, status string, cause error) {
	if repo == nil {
		return
	}
	metadata := dataquery.MetadataFromContext(ctx)
	payload := map[string]any{}
	if cause != nil {
		payload["error"] = cause.Error()
	}
	bytes, _ := json.Marshal(payload)
	_ = access.PersistAuditEvent(ctx, repo, access.AuditEventInput{
		WorkspaceID:   scope.WorkspaceID,
		PrincipalID:   scope.PrincipalID,
		Action:        "agent_tool.called",
		TargetType:    targetType,
		TargetID:      targetID,
		Privilege:     privilege,
		Status:        status,
		RequestID:     metadata.RequestID,
		CorrelationID: metadata.CorrelationID,
		MetadataJSON:  string(bytes),
	})
}

func agentCredentialAllowsPrivilege(scope agentcap.Scope, privilege access.Privilege) bool {
	credential := scope.Credential
	if credential.WorkspaceID != "" && credential.WorkspaceID != scope.WorkspaceID {
		return false
	}
	if !credential.Restricted {
		return true
	}
	for _, allowed := range credential.Privileges {
		if allowed == string(privilege) {
			return true
		}
	}
	return false
}
