package app

import (
	"net/http"

	accessmodule "github.com/Yacobolo/leapview/internal/access/module"
	agentmodule "github.com/Yacobolo/leapview/internal/agent/module"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	apitransport "github.com/Yacobolo/leapview/internal/api/transport"
	dashboardmodule "github.com/Yacobolo/leapview/internal/dashboard/module"
	deploymentmodule "github.com/Yacobolo/leapview/internal/deployment/module"
	manageddatamodule "github.com/Yacobolo/leapview/internal/manageddata/module"
	refreshmodule "github.com/Yacobolo/leapview/internal/refresh/module"
	releasemodule "github.com/Yacobolo/leapview/internal/release/module"
	workspacemodule "github.com/Yacobolo/leapview/internal/workspace/module"
	"github.com/go-chi/chi/v5"
)

func registerAPIGenRoutes(routes *capabilityRoutes, runtime *runtimeServices, platform *platformServices, policy *httpPolicy, r chi.Router) {
	apigenapi.RegisterAPIGenRoutes(r, apiGenRouteHandler{handler: platform.apiGenHandler})
}

type apiGenOperationHandler interface {
	HandleAPIGen(string, http.ResponseWriter, *http.Request)
}

type apiGenRouteHandler struct {
	handler apiGenOperationHandler
}

func (a apiGenRouteHandler) HandleAPIGen(operationID string, w http.ResponseWriter, r *http.Request) {
	a.handler.HandleAPIGen(operationID, w, r)
}

type apiGenDispatcher struct {
	accessModule       *accessmodule.Module
	agentModule        *agentmodule.Module
	dashboardModule    *dashboardmodule.Module
	deploymentModule   *deploymentmodule.Module
	managedDataModule  *manageddatamodule.Module
	refreshModule      *refreshmodule.Module
	releaseModule      *releasemodule.Module
	workspaceModule    *workspacemodule.Module
	defaultEnvironment string
	managedDataTus     http.Handler
	queryAuditEvents   http.HandlerFunc
}

func (a apiGenDispatcher) GetInstance(w http.ResponseWriter, _ *http.Request) {
	apitransport.WriteJSON(w, http.StatusOK, apigenapi.InstanceResponse{Environment: a.defaultEnvironment})
}

func (a apiGenDispatcher) GetCurrentPrincipal(w http.ResponseWriter, r *http.Request) {
	a.accessModule.HTTP().GetCurrentPrincipal(w, r)
}

func (a apiGenDispatcher) ListCurrentEffectivePrivileges(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentEffectivePrivilegesParams) {
	a.accessModule.HTTP().ListCurrentEffectivePrivileges(w, r)
}

func (a apiGenDispatcher) ListCurrentAPITokens(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentAPITokensParams) {
	a.accessModule.HTTP().ListCurrentAPITokens(w, r)
}

func (a apiGenDispatcher) CreateCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateCurrentAPITokenHeaders) {
	a.accessModule.HTTP().CreateCurrentAPIToken(w, r)
}

func (a apiGenDispatcher) RevokeCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().RevokeCurrentAPIToken(w, r)
}

func (a apiGenDispatcher) ListCurrentSessions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentSessionsParams) {
	a.accessModule.HTTP().ListCurrentSessions(w, r)
}

func (a apiGenDispatcher) GetActiveManagedDataRevision(w http.ResponseWriter, r *http.Request, project, connection string) {
	a.managedDataModule.HTTP().GetActiveManagedDataRevision(w, r, project, connection)
}

func (a apiGenDispatcher) ListManagedDataRevisions(w http.ResponseWriter, r *http.Request, project, connection string, params apigenapi.GenListManagedDataRevisionsParams) {
	a.managedDataModule.HTTP().ListManagedDataRevisions(w, r, project, connection, params)
}

func (a apiGenDispatcher) GetManagedDataRevision(w http.ResponseWriter, r *http.Request, project, connection, revision string) {
	a.managedDataModule.HTTP().GetManagedDataRevision(w, r, project, connection, revision)
}

func (a apiGenDispatcher) CreateManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection string, headers apigenapi.GenCreateManagedDataUploadSessionHeaders) {
	a.managedDataModule.HTTP().CreateManagedDataUploadSession(w, r, project, connection, headers)
}

func (a apiGenDispatcher) GetManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string) {
	a.managedDataModule.HTTP().GetManagedDataUploadSession(w, r, project, connection, uploadSession)
}

func (a apiGenDispatcher) ListManagedDataUploadSessions(w http.ResponseWriter, r *http.Request, project, connection string, params apigenapi.GenListManagedDataUploadSessionsParams) {
	a.managedDataModule.HTTP().ListManagedDataUploadSessions(w, r, project, connection, params)
}

func (a apiGenDispatcher) CancelManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenCancelManagedDataUploadSessionHeaders) {
	a.managedDataModule.HTTP().CancelManagedDataUploadSession(w, r, project, connection, uploadSession, headers)
}

func (a apiGenDispatcher) FinalizeManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenFinalizeManagedDataUploadSessionHeaders) {
	a.managedDataModule.HTTP().FinalizeManagedDataUploadSession(w, r, project, connection, uploadSession, headers)
}

func (a apiGenDispatcher) CreateManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenCreateManagedDataS3MultipartUploadHeaders) {
	a.managedDataModule.HTTP().CreateManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, headers)
}

func (a apiGenDispatcher) AbortManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, headers apigenapi.GenAbortManagedDataS3MultipartUploadHeaders) {
	a.managedDataModule.HTTP().AbortManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, multipartUpload, headers)
}

func (a apiGenDispatcher) CompleteManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, headers apigenapi.GenCompleteManagedDataS3MultipartUploadHeaders) {
	a.managedDataModule.HTTP().CompleteManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, multipartUpload, headers)
}

func (a apiGenDispatcher) SignManagedDataS3MultipartPart(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, partNumber int32, _ apigenapi.GenSignManagedDataS3MultipartPartHeaders) {
	a.managedDataModule.HTTP().SignManagedDataS3MultipartPart(w, r, project, connection, uploadSession, multipartUpload, partNumber)
}

func (a apiGenDispatcher) RevokeCurrentSession(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().RevokeCurrentSession(w, r)
}

func (a apiGenDispatcher) ListWorkspaces(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListWorkspacesParams) {
	a.workspaceModule.HTTP().Workspaces(w, r)
}

func (a apiGenDispatcher) Search(w http.ResponseWriter, r *http.Request, params apigenapi.GenSearchParams) {
	a.workspaceModule.SearchAPI(w, r, params)
}

func (a apiGenDispatcher) ListWorkspaceAssets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetsParams) {
	a.workspaceModule.HTTP().Assets(w, r)
}

func (a apiGenDispatcher) GetWorkspaceActiveAssetGraph(w http.ResponseWriter, r *http.Request, _ string) {
	a.workspaceModule.HTTP().ActiveDeploymentGraph(w, r)
}

func (a apiGenDispatcher) GetWorkspaceAsset(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.workspaceModule.HTTP().Asset(w, r)
}

func (a apiGenDispatcher) GetWorkspaceAssetLineage(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.workspaceModule.HTTP().AssetLineage(w, r)
}

func (a apiGenDispatcher) ListWorkspaceAssetEdges(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetEdgesParams) {
	a.workspaceModule.HTTP().AssetEdges(w, r)
}

func (a apiGenDispatcher) ListDashboards(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDashboardsParams) {
	a.dashboardModule.HTTP().ListDashboards(w, r)
}

func (a apiGenDispatcher) GetDashboard(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.dashboardModule.HTTP().GetDashboard(w, r)
}

func (a apiGenDispatcher) GetDashboardPage(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.dashboardModule.HTTP().GetDashboardPage(w, r)
}

func (a apiGenDispatcher) GetDashboardFilter(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.dashboardModule.HTTP().GetDashboardFilter(w, r)
}

func (a apiGenDispatcher) GetDashboardVisual(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.dashboardModule.HTTP().GetDashboardVisual(w, r)
}

func (a apiGenDispatcher) ListSemanticDatasets(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticDatasetsParams) {
	a.dashboardModule.SemanticAPI().ListSemanticDatasets(w, r)
}

func (a apiGenDispatcher) GetSemanticDataset(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.dashboardModule.SemanticAPI().GetSemanticDataset(w, r)
}

func (a apiGenDispatcher) ListSemanticModelFields(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticModelFieldsParams) {
	a.dashboardModule.SemanticAPI().ListSemanticModelFields(w, r)
}

func (a apiGenDispatcher) ListSemanticRelationships(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticRelationshipsParams) {
	a.dashboardModule.SemanticAPI().ListSemanticRelationships(w, r)
}

func (a apiGenDispatcher) ListSemanticSources(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticSourcesParams) {
	a.dashboardModule.SemanticAPI().ListSemanticSources(w, r)
}

func (a apiGenDispatcher) QuerySemanticModel(w http.ResponseWriter, r *http.Request, workspaceID, _ string, headers apigenapi.GenQuerySemanticModelHeaders) {
	a.dashboardModule.QuerySemanticModel(w, r, workspaceID, headers)
}

func (a apiGenDispatcher) ExplainSemanticModelQuery(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.dashboardModule.SemanticAPI().ExplainSemanticModelQuery(w, r)
}

func (a apiGenDispatcher) ListSemanticFields(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListSemanticFieldsParams) {
	a.dashboardModule.SemanticAPI().ListSemanticFields(w, r)
}

func (a apiGenDispatcher) PreviewSemanticDataset(w http.ResponseWriter, r *http.Request, workspaceID, _, _ string, headers apigenapi.GenPreviewSemanticDatasetHeaders) {
	a.dashboardModule.PreviewSemanticDataset(w, r, workspaceID, headers)
}

func (a apiGenDispatcher) ExplainSemanticPreview(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.dashboardModule.SemanticAPI().ExplainSemanticPreview(w, r)
}

func (a apiGenDispatcher) QueryDashboardPage(w http.ResponseWriter, r *http.Request, workspaceID, _, _ string) {
	a.dashboardModule.QueryDashboardPage(w, r, workspaceID)
}

func (a apiGenDispatcher) QueryDashboardVisualData(w http.ResponseWriter, r *http.Request, workspaceID, _, _, _ string, headers apigenapi.GenQueryDashboardVisualDataHeaders) {
	a.dashboardModule.QueryDashboardVisualData(w, r, workspaceID, headers)
}

func (a apiGenDispatcher) ListDashboardFilterValues(w http.ResponseWriter, r *http.Request, workspaceID, _, _, _ string, params apigenapi.GenListDashboardFilterValuesParams) {
	a.dashboardModule.ListDashboardFilterValues(w, r, workspaceID, params)
}

func (a apiGenDispatcher) CreateRefreshRun(w http.ResponseWriter, r *http.Request, workspaceID string, headers apigenapi.GenCreateRefreshRunHeaders) {
	a.refreshModule.CreateRefreshRun(w, r, workspaceID, headers)
}

func (a apiGenDispatcher) ListRefreshRuns(w http.ResponseWriter, r *http.Request, workspaceID string, params apigenapi.GenListRefreshRunsParams) {
	a.refreshModule.ListRefreshRuns(w, r, workspaceID, params)
}

func (a apiGenDispatcher) GetRefreshRun(w http.ResponseWriter, r *http.Request, workspaceID, runID string) {
	a.refreshModule.GetRefreshRun(w, r, workspaceID, runID)
}

func (a apiGenDispatcher) ListAgentConversations(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListAgentConversationsParams) {
	a.agentModule.HTTP().ListConversations(w, r)
}

func (a apiGenDispatcher) CreateAgentConversation(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateAgentConversationHeaders) {
	a.agentModule.HTTP().CreateConversation(w, r)
}

func (a apiGenDispatcher) GetAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.agentModule.HTTP().GetConversation(w, r)
}

func (a apiGenDispatcher) UpdateAgentConversation(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdateAgentConversationHeaders) {
	a.agentModule.UpdateConversation(w, r, headers)
}

func (a apiGenDispatcher) ArchiveAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.agentModule.HTTP().ArchiveConversation(w, r)
}

func (a apiGenDispatcher) ListAgentMessages(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentMessagesParams) {
	a.agentModule.HTTP().ListMessages(w, r)
}

func (a apiGenDispatcher) CreateAgentRun(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateAgentRunHeaders) {
	a.agentModule.HTTP().CreateRun(w, r)
}

func (a apiGenDispatcher) ListAgentRuns(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentRunsParams) {
	a.agentModule.HTTP().ListRuns(w, r)
}

func (a apiGenDispatcher) GetAgentRun(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.agentModule.HTTP().GetRun(w, r)
}

func (a apiGenDispatcher) ListAgentEvents(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentEventsParams, _ apigenapi.GenListAgentEventsHeaders) {
	a.agentModule.HTTP().ListEvents(w, r)
}

func (a apiGenDispatcher) CancelAgentRun(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenCancelAgentRunHeaders) {
	a.agentModule.HTTP().CancelRun(w, r)
}

func (a apiGenDispatcher) ListPrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListPrincipalsParams) {
	a.accessModule.HTTP().ListPrincipals(w, r)
}

func (a apiGenDispatcher) CreatePrincipal(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreatePrincipalHeaders) {
	a.accessModule.HTTP().CreatePrincipal(w, r)
}

func (a apiGenDispatcher) GetPrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().GetPrincipal(w, r)
}

func (a apiGenDispatcher) UpdatePrincipal(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdatePrincipalHeaders) {
	a.accessModule.UpdatePrincipal(w, r, headers)
}

func (a apiGenDispatcher) DeletePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().DeletePrincipal(w, r)
}

func (a apiGenDispatcher) ResetPrincipalPassword(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenResetPrincipalPasswordHeaders) {
	a.accessModule.HTTP().ResetPrincipalPassword(w, r)
}

func (a apiGenDispatcher) ListServicePrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListServicePrincipalsParams) {
	a.accessModule.HTTP().ListServicePrincipals(w, r)
}

func (a apiGenDispatcher) CreateServicePrincipal(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateServicePrincipalHeaders) {
	a.accessModule.HTTP().CreateServicePrincipal(w, r)
}

func (a apiGenDispatcher) GetServicePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().GetServicePrincipal(w, r)
}

func (a apiGenDispatcher) UpdateServicePrincipal(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdateServicePrincipalHeaders) {
	a.accessModule.UpdateServicePrincipal(w, r, headers)
}

func (a apiGenDispatcher) DeleteServicePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().DeleteServicePrincipal(w, r)
}

func (a apiGenDispatcher) CreateServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateServicePrincipalSecretHeaders) {
	a.accessModule.HTTP().CreateServicePrincipalSecret(w, r)
}

func (a apiGenDispatcher) ListServicePrincipalSecrets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListServicePrincipalSecretsParams) {
	a.accessModule.HTTP().ListServicePrincipalSecrets(w, r)
}

func (a apiGenDispatcher) GetServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().GetServicePrincipalSecret(w, r)
}

func (a apiGenDispatcher) RevokeServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().RevokeServicePrincipalSecret(w, r)
}

func (a apiGenDispatcher) ListWorkspaceRoles(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceRolesParams) {
	a.accessModule.HTTP().ListWorkspaceRoles(w, r)
}

func (a apiGenDispatcher) ListEffectivePrivileges(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListEffectivePrivilegesParams) {
	a.accessModule.HTTP().ListEffectivePrivileges(w, r)
}

func (a apiGenDispatcher) ListGrants(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListGrantsParams) {
	a.accessModule.HTTP().ListGrants(w, r)
}

func (a apiGenDispatcher) CreateGrant(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateGrantHeaders) {
	a.accessModule.HTTP().CreateGrant(w, r)
}

func (a apiGenDispatcher) GetGrant(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().GetGrant(w, r)
}

func (a apiGenDispatcher) UpdateGrant(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateGrantHeaders) {
	a.accessModule.UpdateGrant(w, r, headers)
}

func (a apiGenDispatcher) DeleteGrant(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().DeleteGrant(w, r)
}

func (a apiGenDispatcher) ListDataPolicies(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDataPoliciesParams) {
	a.accessModule.HTTP().ListDataPolicies(w, r)
}

func (a apiGenDispatcher) CreateDataPolicy(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateDataPolicyHeaders) {
	a.accessModule.HTTP().CreateDataPolicy(w, r)
}

func (a apiGenDispatcher) GetDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().GetDataPolicy(w, r)
}

func (a apiGenDispatcher) UpdateDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateDataPolicyHeaders) {
	a.accessModule.UpdateDataPolicy(w, r, headers)
}

func (a apiGenDispatcher) CheckAuthorizationBatch(w http.ResponseWriter, r *http.Request, _ string) {
	a.accessModule.HTTP().CheckAuthorizationBatch(w, r)
}

func (a apiGenDispatcher) DeleteDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().DeleteDataPolicy(w, r)
}

func (a apiGenDispatcher) TransferOwnership(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenTransferOwnershipHeaders) {
	a.accessModule.HTTP().TransferOwnership(w, r)
}

func (a apiGenDispatcher) ListSemanticModels(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListSemanticModelsParams) {
	a.dashboardModule.SemanticAPI().ListSemanticModels(w, r)
}

func (a apiGenDispatcher) GetSemanticModel(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.dashboardModule.SemanticAPI().GetSemanticModel(w, r)
}

func (a apiGenDispatcher) ListGroups(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListGroupsParams) {
	a.accessModule.HTTP().ListGroups(w, r)
}

func (a apiGenDispatcher) CreateGroup(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateGroupHeaders) {
	a.accessModule.HTTP().CreateGroup(w, r)
}

func (a apiGenDispatcher) GetGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().GetGroup(w, r)
}

func (a apiGenDispatcher) UpdateGroup(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateGroupHeaders) {
	a.accessModule.UpdateGroup(w, r, headers)
}

func (a apiGenDispatcher) DeleteGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().DeleteGroup(w, r)
}

func (a apiGenDispatcher) ListGroupMembers(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListGroupMembersParams) {
	a.accessModule.HTTP().ListGroupMembers(w, r)
}

func (a apiGenDispatcher) AddGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.accessModule.HTTP().AddGroupMember(w, r)
}

func (a apiGenDispatcher) RemoveGroupMember(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.accessModule.HTTP().RemoveGroupMember(w, r)
}

func (a apiGenDispatcher) ListRoleBindings(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListRoleBindingsParams) {
	a.accessModule.HTTP().ListRoleBindings(w, r)
}

func (a apiGenDispatcher) CreateRoleBinding(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateRoleBindingHeaders) {
	a.accessModule.HTTP().CreateRoleBinding(w, r)
}

func (a apiGenDispatcher) GetRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().GetRoleBinding(w, r)
}

func (a apiGenDispatcher) UpdateRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateRoleBindingHeaders) {
	a.accessModule.UpdateRoleBinding(w, r, headers)
}

func (a apiGenDispatcher) DeleteRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.accessModule.HTTP().DeleteRoleBinding(w, r)
}

func (a apiGenDispatcher) ListAuditEvents(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAuditEventsParams) {
	a.accessModule.HTTP().ListAuditEvents(w, r)
}

func (a apiGenDispatcher) ListQueryEvents(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListQueryEventsParams) {
	a.queryAuditEvents(w, r)
}
