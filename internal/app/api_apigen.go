package app

import (
	"bytes"
	"encoding/json"
	"net/http"

	apigenapi "github.com/Yacobolo/libredash/internal/api/gen"
	"github.com/go-chi/chi/v5"
)

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
		buffered := newAPIGenResponseBuffer(w)
		if ok := apigenapi.DispatchAPIGenOperation(operationID, a, buffered, r); !ok {
			http.NotFound(w, r)
			return
		}
		buffered.flush()
	})).ServeHTTP(w, r)
}

type apiGenResponseBuffer struct {
	downstream http.ResponseWriter
	header     http.Header
	body       bytes.Buffer
	status     int
}

func newAPIGenResponseBuffer(w http.ResponseWriter) *apiGenResponseBuffer {
	return &apiGenResponseBuffer{downstream: w, header: http.Header{}}
}

func (w *apiGenResponseBuffer) Header() http.Header {
	return w.header
}

func (w *apiGenResponseBuffer) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *apiGenResponseBuffer) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *apiGenResponseBuffer) flush() {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	for key, values := range w.header {
		for _, value := range values {
			w.downstream.Header().Add(key, value)
		}
	}
	body := w.normalizedBody(status)
	w.downstream.WriteHeader(status)
	_, _ = w.downstream.Write(body)
}

func (w *apiGenResponseBuffer) normalizedBody(status int) []byte {
	if status < 400 || w.body.Len() == 0 {
		return w.body.Bytes()
	}
	var value map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &value); err != nil {
		return w.body.Bytes()
	}
	if _, ok := value["code"]; !ok {
		return w.body.Bytes()
	}
	if _, ok := value["message"]; !ok {
		return w.body.Bytes()
	}
	if _, ok := value["details"]; !ok {
		value["details"] = map[string]any{}
	}
	if _, ok := value["requestId"]; !ok {
		value["requestId"] = ""
	}
	out, err := json.Marshal(value)
	if err != nil {
		return w.body.Bytes()
	}
	return append(out, '\n')
}

func (a apiGenAdapter) GetCurrentPrincipal(w http.ResponseWriter, r *http.Request) {
	a.server.accessHTTPHandler().GetCurrentPrincipal(w, r)
}

func (a apiGenAdapter) ListCurrentPermissions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentPermissionsParams) {
	a.server.accessHTTPHandler().ListCurrentPermissions(w, r)
}

func (a apiGenAdapter) ListCurrentAPITokens(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentAPITokensParams) {
	a.server.accessHTTPHandler().ListCurrentAPITokens(w, r)
}

func (a apiGenAdapter) CreateCurrentAPIToken(w http.ResponseWriter, r *http.Request) {
	a.server.accessHTTPHandler().CreateCurrentAPIToken(w, r)
}

func (a apiGenAdapter) RevokeCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().RevokeCurrentAPIToken(w, r)
}

func (a apiGenAdapter) ListCurrentSessions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentSessionsParams) {
	a.server.accessHTTPHandler().ListCurrentSessions(w, r)
}

func (a apiGenAdapter) RevokeCurrentSession(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().RevokeCurrentSession(w, r)
}

func (a apiGenAdapter) ListWorkspaces(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListWorkspacesParams) {
	a.server.workspaceHTTPHandler().Workspaces(w, r)
}

func (a apiGenAdapter) SearchWorkspace(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenSearchWorkspaceParams) {
	a.server.workspaceHTTPHandler().SearchWorkspace(w, r)
}

func (a apiGenAdapter) ListWorkspaceAssets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetsParams) {
	a.server.workspaceHTTPHandler().Assets(w, r)
}

func (a apiGenAdapter) GetWorkspaceActiveDeploymentGraph(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenGetWorkspaceActiveDeploymentGraphParams) {
	a.server.workspaceHTTPHandler().ActiveDeploymentGraph(w, r)
}

func (a apiGenAdapter) GetWorkspaceAsset(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.workspaceHTTPHandler().Asset(w, r)
}

func (a apiGenAdapter) GetWorkspaceAssetLineage(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.workspaceHTTPHandler().AssetLineage(w, r)
}

func (a apiGenAdapter) ListWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetEdgesParams) {
	a.server.workspaceHTTPHandler().AssetEdges(w, r)
}

func (a apiGenAdapter) ListDashboards(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDashboardsParams) {
	a.server.dashboardHTTP().ListDashboards(w, r)
}

func (a apiGenAdapter) GetDashboard(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.dashboardHTTP().GetDashboard(w, r)
}

func (a apiGenAdapter) ListDashboardComponents(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListDashboardComponentsParams) {
	a.server.dashboardHTTP().ListDashboardComponents(w, r)
}

func (a apiGenAdapter) GetDashboardVisual(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.server.dashboardHTTP().GetDashboardVisual(w, r)
}

func (a apiGenAdapter) ListSemanticDatasets(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticDatasetsParams) {
	a.server.semanticQueryHTTP().ListSemanticDatasets(w, r)
}

func (a apiGenAdapter) GetSemanticDataset(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().GetSemanticDataset(w, r)
}

func (a apiGenAdapter) ListSemanticFields(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListSemanticFieldsParams) {
	a.server.semanticQueryHTTP().ListSemanticFields(w, r)
}

func (a apiGenAdapter) QuerySemanticDataset(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().QuerySemanticDataset(w, r)
}

func (a apiGenAdapter) PreviewSemanticDataset(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().PreviewSemanticDataset(w, r)
}

func (a apiGenAdapter) ExplainSemanticQuery(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().ExplainSemanticQuery(w, r)
}

func (a apiGenAdapter) ExplainSemanticPreview(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().ExplainSemanticPreview(w, r)
}

func (a apiGenAdapter) QueryDashboardPage(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.dashboardHTTP().QueryDashboardPage(w, r)
}

func (a apiGenAdapter) QueryDashboardVisualData(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.server.dashboardHTTP().QueryDashboardVisualData(w, r)
}

func (a apiGenAdapter) QueryDashboardTable(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.dashboardHTTP().QueryDashboardTable(w, r)
}

func (a apiGenAdapter) QueryDashboardTableData(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.server.dashboardHTTP().QueryDashboardTableData(w, r)
}

func (a apiGenAdapter) ListDashboardFilterOptions(w http.ResponseWriter, r *http.Request, _, _, _, _ string, _ apigenapi.GenListDashboardFilterOptionsParams) {
	a.server.dashboardHTTP().ListDashboardFilterOptions(w, r)
}

func (a apiGenAdapter) ListDeployments(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDeploymentsParams) {
	a.server.deploymentHTTPHandler().List(w, r)
}

func (a apiGenAdapter) CreateDeployment(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.deploymentHTTPHandler().Create(w, r)
}

func (a apiGenAdapter) GetDeployment(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenGetDeploymentParams) {
	a.server.deploymentHTTPHandler().Get(w, r)
}

func (a apiGenAdapter) UploadDeploymentArtifact(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenUploadDeploymentArtifactHeaders) {
	a.server.deploymentHTTPHandler().UploadArtifact(w, r)
}

func (a apiGenAdapter) ActivateDeployment(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenActivateDeploymentParams) {
	a.server.deploymentHTTPHandler().Activate(w, r)
}

func (a apiGenAdapter) ValidateDeployment(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenValidateDeploymentParams) {
	a.server.deploymentHTTPHandler().Validate(w, r)
}

func (a apiGenAdapter) CreateMaterializationRun(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.materializationHTTP().CreateRun(w, r)
}

func (a apiGenAdapter) ListMaterializationRuns(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListMaterializationRunsParams) {
	a.server.materializationHTTP().ListRuns(w, r)
}

func (a apiGenAdapter) GetMaterializationRun(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.materializationHTTP().GetRun(w, r)
}

func (a apiGenAdapter) ListAgentConversations(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentConversationsParams) {
	a.server.agentHTTPHandler().ListConversations(w, r)
}

func (a apiGenAdapter) CreateAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.agentHTTPHandler().CreateConversation(w, r)
}

func (a apiGenAdapter) GetAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.agentHTTPHandler().GetConversation(w, r)
}

func (a apiGenAdapter) UpdateAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.agentHTTPHandler().UpdateConversation(w, r)
}

func (a apiGenAdapter) ArchiveAgentConversation(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.agentHTTPHandler().ArchiveConversation(w, r)
}

func (a apiGenAdapter) ListAgentMessages(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentMessagesParams) {
	a.server.agentHTTPHandler().ListMessages(w, r)
}

func (a apiGenAdapter) CreateAgentTurn(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.agentHTTPHandler().CreateTurn(w, r)
}

func (a apiGenAdapter) ListAgentRuns(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentRunsParams) {
	a.server.agentHTTPHandler().ListRuns(w, r)
}

func (a apiGenAdapter) GetAgentRun(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.agentHTTPHandler().GetRun(w, r)
}

func (a apiGenAdapter) ListAgentEvents(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListAgentEventsParams) {
	a.server.agentHTTPHandler().ListEvents(w, r)
}

func (a apiGenAdapter) GetAdminAgentConfig(w http.ResponseWriter, r *http.Request) {
	a.server.agentHTTPHandler().GetAdminConfig(w, r)
}

func (a apiGenAdapter) UpdateAdminAgentConfig(w http.ResponseWriter, r *http.Request) {
	a.server.agentHTTPHandler().UpdateAdminConfig(w, r)
}

func (a apiGenAdapter) ListPrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListPrincipalsParams) {
	a.server.accessHTTPHandler().ListPrincipals(w, r)
}

func (a apiGenAdapter) GetPrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().GetPrincipal(w, r)
}

func (a apiGenAdapter) UpdatePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().UpdatePrincipal(w, r)
}

func (a apiGenAdapter) ListWorkspaceRoles(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceRolesParams) {
	a.server.accessHTTPHandler().ListWorkspaceRoles(w, r)
}

func (a apiGenAdapter) ListSemanticModels(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListSemanticModelsParams) {
	a.server.semanticQueryHTTP().ListSemanticModels(w, r)
}

func (a apiGenAdapter) GetSemanticModel(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.semanticQueryHTTP().GetSemanticModel(w, r)
}

func (a apiGenAdapter) ListGroups(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListGroupsParams) {
	a.server.accessHTTPHandler().ListGroups(w, r)
}

func (a apiGenAdapter) CreateGroup(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().CreateGroup(w, r)
}

func (a apiGenAdapter) GetGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetGroup(w, r)
}

func (a apiGenAdapter) UpdateGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().UpdateGroup(w, r)
}

func (a apiGenAdapter) DeleteGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().DeleteGroup(w, r)
}

func (a apiGenAdapter) ListGroupMembers(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListGroupMembersParams) {
	a.server.accessHTTPHandler().ListGroupMembers(w, r)
}

func (a apiGenAdapter) AddGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.accessHTTPHandler().AddGroupMember(w, r)
}

func (a apiGenAdapter) RemoveGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.accessHTTPHandler().RemoveGroupMember(w, r)
}

func (a apiGenAdapter) ListRoleBindings(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListRoleBindingsParams) {
	a.server.accessHTTPHandler().ListRoleBindings(w, r)
}

func (a apiGenAdapter) CreateRoleBinding(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().CreateRoleBinding(w, r)
}

func (a apiGenAdapter) UpdateRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().UpdateRoleBinding(w, r)
}

func (a apiGenAdapter) DeleteRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().DeleteRoleBinding(w, r)
}

func (a apiGenAdapter) ListAuditEvents(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAuditEventsParams) {
	a.server.accessHTTPHandler().ListAuditEvents(w, r)
}

func (a apiGenAdapter) ListQueryEvents(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListQueryEventsParams) {
	a.server.accessHTTPHandler().ListQueryEvents(w, r)
}
