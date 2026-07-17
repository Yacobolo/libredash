package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/Yacobolo/libredash/internal/access"
	agentcap "github.com/Yacobolo/libredash/internal/agent"
	agenttools "github.com/Yacobolo/libredash/internal/agent/tools"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
)

func (s *Server) configureAgentTools() {
	if s.agent == nil {
		return
	}
	if s.store != nil {
		s.agent.SetSystemPromptProvider(s.agentSystemPrompt)
	}
	s.agent.SetPolicyProvider(s.agentPolicyForScope)
	visualProvider := s.agentVisualToolProvider()
	apigenProvider := s.agentAPIGenToolProvider()
	s.agent.AppendToolProviders(
		func(scope agentcap.Scope) []agentcore.ToolDefinition {
			return visualProvider.Definitions(agentToolsScope(scope))
		},
		func(scope agentcap.Scope) []agentcore.ToolDefinition {
			return apigenProvider.Definitions(agentToolsScope(scope))
		},
	)
}

func (s *Server) agentPolicyForScope(scope agentcap.Scope) (workspace.AgentPolicy, bool) {
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
		SemanticModel: func(modelID string) (model *semanticmodel.Model, ok bool) {
			if s.metrics == nil {
				return nil, false
			}
			return s.metrics.SemanticModel(modelID)
		},
		AggregateRows: func(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
			return executeAggregateRows(ctx, s.metrics, modelID, request)
		},
		PreviewRows: func(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
			return executePreviewRows(ctx, s.metrics, modelID, request)
		},
	}
}

func (s *Server) agentAPIGenToolProvider() agenttools.APIGenProvider {
	return agenttools.APIGenProvider{
		Authorize: func(ctx context.Context, scope agenttools.Scope, operationID string) (agentcore.ToolResult, bool) {
			return s.authorizeAPIGenAgentOperation(ctx, agentScopeFromTools(scope), operationID)
		},
		Dispatch: func(operationID string, request *http.Request) (*http.Response, bool) {
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
		return agenttools.ToolError("forbidden", "operation has no generated LibreDash privilege metadata"), false
	}
	return s.authorizeAgentPrivilege(ctx, scope, privilege, []access.ObjectRef{access.WorkspaceObject(scope.WorkspaceID)}, "agent_tool", operationID)
}

func (s *Server) authorizeAgentPrivilege(ctx context.Context, scope agentcap.Scope, privilege access.Privilege, objects []access.ObjectRef, targetType, targetID string) (agentcore.ToolResult, bool) {
	if scope.PrincipalID == "" {
		return agenttools.ToolError("unauthorized", "agent tool requires an authenticated principal"), false
	}
	if !agentCredentialAllowsPrivilege(scope, privilege) {
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
