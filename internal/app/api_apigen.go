package app

import (
	"net/http"

	"github.com/Yacobolo/libredash/internal/access"
	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/go-chi/chi/v5"
)

var apigenOperationPermissions = map[string]string{
	"getCurrentPrincipal":      access.PermissionWorkspaceRead,
	"listCurrentPermissions":   access.PermissionWorkspaceRead,
	"listCurrentAPITokens":     access.PermissionTokenManage,
	"createCurrentAPIToken":    access.PermissionTokenManage,
	"revokeCurrentAPIToken":    access.PermissionTokenManage,
	"listCurrentSessions":      access.PermissionWorkspaceRead,
	"revokeCurrentSession":     access.PermissionWorkspaceRead,
	"listWorkspaces":           access.PermissionWorkspaceRead,
	"listWorkspaceAssets":      access.PermissionAssetRead,
	"listWorkspaceAssetEdges":  access.PermissionAssetRead,
	"createDeployment":         access.PermissionDeploymentWrite,
	"listDeployments":          access.PermissionDeploymentRead,
	"getDeployment":            access.PermissionDeploymentRead,
	"uploadDeploymentArtifact": access.PermissionDeploymentWrite,
	"validateDeployment":       access.PermissionDeploymentWrite,
	"activateDeployment":       access.PermissionDeploymentActivate,
	"createMaterializationRun": access.PermissionMaterializationRun,
	"listMaterializationRuns":  access.PermissionMaterializationRun,
	"getMaterializationRun":    access.PermissionMaterializationRun,
	"createAgentConversation":  access.PermissionAgentUse,
	"listAgentConversations":   access.PermissionAgentRead,
	"getAgentConversation":     access.PermissionAgentRead,
	"updateAgentConversation":  access.PermissionAgentUse,
	"archiveAgentConversation": access.PermissionAgentUse,
	"listAgentMessages":        access.PermissionAgentRead,
	"createAgentTurn":          access.PermissionAgentUse,
	"listAgentRuns":            access.PermissionAgentRead,
	"getAgentRun":              access.PermissionAgentRead,
	"listAgentEvents":          access.PermissionAgentRead,
	"listPrincipals":           access.PermissionRBACRead,
	"getPrincipal":             access.PermissionRBACRead,
	"updatePrincipal":          access.PermissionRBACWrite,
	"listWorkspaceRoles":       access.PermissionRBACRead,
	"listGroups":               access.PermissionRBACRead,
	"createGroup":              access.PermissionRBACWrite,
	"getGroup":                 access.PermissionRBACRead,
	"updateGroup":              access.PermissionRBACWrite,
	"deleteGroup":              access.PermissionRBACWrite,
	"listGroupMembers":         access.PermissionRBACRead,
	"addGroupMember":           access.PermissionRBACWrite,
	"removeGroupMember":        access.PermissionRBACWrite,
	"listRoleBindings":         access.PermissionRBACRead,
	"createRoleBinding":        access.PermissionRBACWrite,
	"updateRoleBinding":        access.PermissionRBACWrite,
	"deleteRoleBinding":        access.PermissionRBACWrite,
	"listAuditEvents":          access.PermissionAuditRead,
}

func (s *Server) registerAPIGenRoutes(r chi.Router) {
	apigenapi.RegisterAPIGenRoutes(r, apiGenAdapter{server: s})
}

type apiGenAdapter struct {
	server *Server
}

func (a apiGenAdapter) HandleAPIGen(operationID string, w http.ResponseWriter, r *http.Request) {
	permission, ok := apigenOperationPermissions[operationID]
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.server.protect(permission, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok := apigenapi.DispatchAPIGenOperation(operationID, a, w, r); !ok {
			http.NotFound(w, r)
		}
	})).ServeHTTP(w, r)
}

func (a apiGenAdapter) GetCurrentPrincipal(w http.ResponseWriter, r *http.Request) {
	a.server.apiGetCurrentPrincipal(w, r)
}

func (a apiGenAdapter) ListCurrentPermissions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentPermissionsParams) {
	a.server.apiListCurrentPermissions(w, r)
}

func (a apiGenAdapter) ListCurrentAPITokens(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentAPITokensParams) {
	a.server.apiListCurrentAPITokens(w, r)
}

func (a apiGenAdapter) CreateCurrentAPIToken(w http.ResponseWriter, r *http.Request) {
	a.server.apiCreateCurrentAPIToken(w, r)
}

func (a apiGenAdapter) RevokeCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiRevokeCurrentAPIToken(w, r)
}

func (a apiGenAdapter) ListCurrentSessions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentSessionsParams) {
	a.server.apiListCurrentSessions(w, r)
}

func (a apiGenAdapter) RevokeCurrentSession(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiRevokeCurrentSession(w, r)
}

func (a apiGenAdapter) ListWorkspaces(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListWorkspacesParams) {
	a.server.apiWorkspaces(w, r)
}

func (a apiGenAdapter) ListWorkspaceAssets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetsParams) {
	a.server.apiWorkspaceAssets(w, r)
}

func (a apiGenAdapter) ListWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetEdgesParams) {
	a.server.apiWorkspaceAssetEdges(w, r)
}

func (a apiGenAdapter) ListDeployments(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDeploymentsParams) {
	a.server.listDeployments(w, r)
}

func (a apiGenAdapter) CreateDeployment(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.createDeployment(w, r)
}

func (a apiGenAdapter) GetDeployment(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.getDeployment(w, r)
}

func (a apiGenAdapter) UploadDeploymentArtifact(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.uploadDeploymentArtifact(w, r)
}

func (a apiGenAdapter) ActivateDeployment(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.activateDeployment(w, r)
}

func (a apiGenAdapter) ValidateDeployment(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.validateDeployment(w, r)
}

func (a apiGenAdapter) CreateMaterializationRun(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.createMaterializationRun(w, r)
}

func (a apiGenAdapter) ListMaterializationRuns(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListMaterializationRunsParams) {
	a.server.listMaterializationRuns(w, r)
}

func (a apiGenAdapter) GetMaterializationRun(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.getMaterializationRun(w, r)
}

func (a apiGenAdapter) ListAgentConversations(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentConversationsParams) {
	a.server.listAgentConversations(w, r)
}

func (a apiGenAdapter) CreateAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.createAgentConversation(w, r)
}

func (a apiGenAdapter) GetAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.getAgentConversation(w, r)
}

func (a apiGenAdapter) UpdateAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.updateAgentConversation(w, r)
}

func (a apiGenAdapter) ArchiveAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.archiveAgentConversation(w, r)
}

func (a apiGenAdapter) ListAgentMessages(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentMessagesParams) {
	a.server.listAgentMessages(w, r)
}

func (a apiGenAdapter) CreateAgentTurn(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.createAgentTurn(w, r)
}

func (a apiGenAdapter) ListAgentRuns(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentRunsParams) {
	a.server.listAgentRuns(w, r)
}

func (a apiGenAdapter) GetAgentRun(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.getAgentRun(w, r)
}

func (a apiGenAdapter) ListAgentEvents(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListAgentEventsParams) {
	a.server.listAgentEvents(w, r)
}

func (a apiGenAdapter) ListPrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListPrincipalsParams) {
	a.server.apiListPrincipals(w, r)
}

func (a apiGenAdapter) GetPrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiGetPrincipal(w, r)
}

func (a apiGenAdapter) UpdatePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiUpdatePrincipal(w, r)
}

func (a apiGenAdapter) ListWorkspaceRoles(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceRolesParams) {
	a.server.apiWorkspaceRoles(w, r)
}

func (a apiGenAdapter) ListGroups(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListGroupsParams) {
	a.server.apiListGroups(w, r)
}

func (a apiGenAdapter) CreateGroup(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiCreateGroup(w, r)
}

func (a apiGenAdapter) GetGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.apiGetGroup(w, r)
}

func (a apiGenAdapter) UpdateGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.apiUpdateGroup(w, r)
}

func (a apiGenAdapter) DeleteGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.apiDeleteGroup(w, r)
}

func (a apiGenAdapter) ListGroupMembers(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListGroupMembersParams) {
	a.server.apiListGroupMembers(w, r)
}

func (a apiGenAdapter) AddGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.apiAddGroupMember(w, r)
}

func (a apiGenAdapter) RemoveGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.apiRemoveGroupMember(w, r)
}

func (a apiGenAdapter) ListRoleBindings(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListRoleBindingsParams) {
	a.server.apiRoleBindings(w, r)
}

func (a apiGenAdapter) CreateRoleBinding(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.apiCreateRoleBinding(w, r)
}

func (a apiGenAdapter) UpdateRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.apiUpdateRoleBinding(w, r)
}

func (a apiGenAdapter) DeleteRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.apiDeleteRoleBinding(w, r)
}

func (a apiGenAdapter) ListAuditEvents(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAuditEventsParams) {
	a.server.apiListAuditEvents(w, r)
}
