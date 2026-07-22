package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/access/httpauth"
	apigenapi "github.com/Yacobolo/leapview/internal/api/gen"
	"github.com/Yacobolo/leapview/internal/workspace"
	"github.com/go-chi/chi/v5"
)

func (s *Server) registerAPIGenRoutes(r chi.Router) {
	apigenapi.RegisterAPIGenRoutes(r, apiGenAdapter{server: s})
}

type apiGenAdapter struct {
	server *Server
}

func (a apiGenAdapter) GetInstance(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, apigenapi.InstanceResponse{Environment: a.server.defaultEnvironment})
}

func (a apiGenAdapter) HandleAPIGen(operationID string, w http.ResponseWriter, r *http.Request) {
	contract, ok := apigenapi.GetAPIGenOperationContract(operationID)
	if !ok || !contract.Protected {
		http.NotFound(w, r)
		return
	}
	var privilege access.Privilege
	if contract.AuthzMode == "privilege" {
		privilege, ok = apigenOperationPrivilege(operationID)
		if !ok {
			http.NotFound(w, r)
			return
		}
	} else if contract.AuthzMode != "authenticated" {
		http.NotFound(w, r)
		return
	}
	var objectResolver httpauth.ObjectResolver
	if !isGlobalAgentOperation(operationID) {
		objectResolver, ok = apigenOperationObjectResolver(operationID)
		if !ok {
			http.NotFound(w, r)
			return
		}
	}
	protected := a.server.protectWithObjects
	if isGlobalAgentOperation(operationID) {
		protected = func(privilege access.Privilege, _ httpauth.ObjectResolver, next http.Handler) http.Handler {
			return a.server.protectGlobalAgent(privilege, next)
		}
	}
	protected(privilege, objectResolver, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buffered := newAPIGenResponseBuffer(w, r)
		if ok := apigenapi.DispatchAPIGenOperation(operationID, a, apiGenTransportErrorResponder{server: a.server}, buffered, r); !ok {
			http.NotFound(w, r)
			return
		}
		buffered.flush()
	})).ServeHTTP(w, r)
}

func isGlobalAgentOperation(operationID string) bool {
	switch operationID {
	case "search", "listAgentConversations", "createAgentConversation", "archiveAgentConversation", "getAgentConversation", "updateAgentConversation",
		"listAgentMessages", "listAgentRuns", "createAgentRun", "getAgentRun", "cancelAgentRun", "listAgentEvents":
		return true
	default:
		return false
	}
}

func apigenOperationPrivilege(operationID string) (access.Privilege, bool) {
	contract, ok := apigenapi.GetAPIGenOperationContract(operationID)
	if !ok || !contract.Protected || contract.AuthzMode != "privilege" {
		return "", false
	}
	authz, ok := contract.Extensions["x-authz"].(map[string]any)
	if !ok || authz["mode"] != "privilege" {
		return "", false
	}
	value, ok := authz["privilege"].(string)
	if !ok {
		return "", false
	}
	return access.ParsePrivilege(value)
}

type apiGenTransportErrorResponder struct {
	server *Server
}

func (responder apiGenTransportErrorResponder) RespondTransportError(ctx context.Context, w http.ResponseWriter, r *http.Request, failure apigenapi.GenTransportError) {
	if responder.server != nil && responder.server.logger != nil && failure.Cause != nil {
		log := responder.server.logger.DebugContext
		if failure.StatusCode >= http.StatusInternalServerError {
			log = responder.server.logger.ErrorContext
		}
		log(ctx, "APIGen transport error", "operation", failure.OperationID, "kind", failure.Kind, "status", failure.StatusCode, "error", failure.Cause)
	}
	requestID := ""
	instance := ""
	if r != nil {
		requestID = r.Header.Get("X-Request-ID")
		instance = r.URL.Path
	}
	problem := apigenapi.ProblemDetails{
		Type:      "https://leapview.dev/problems/" + strings.ToLower(strings.ReplaceAll(failure.Code, "_", "-")),
		Title:     http.StatusText(failure.StatusCode),
		Status:    int32(failure.StatusCode),
		Detail:    failure.PublicDetail,
		Instance:  instance,
		Code:      failure.Code,
		RequestId: requestID,
		Errors:    []apigenapi.ProblemFieldError{},
	}
	if field := transportErrorField(failure); field != "" {
		problem.Detail = strings.TrimSuffix(failure.PublicDetail, ".") + " \"" + field + "\"."
		problem.Errors = append(problem.Errors, apigenapi.ProblemFieldError{
			Code:   failure.Code,
			Detail: failure.PublicDetail,
			Field:  field,
		})
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(failure.StatusCode)
	_ = json.NewEncoder(w).Encode(problem)
}

func transportErrorField(failure apigenapi.GenTransportError) string {
	if failure.Cause == nil {
		return ""
	}
	switch failure.Kind {
	case "path_parameter", "query_parameter", "header_parameter":
	default:
		return ""
	}
	message := failure.Cause.Error()
	marker := "parameter \""
	start := strings.Index(message, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.IndexByte(message[start:], '"')
	if end < 0 {
		return ""
	}
	return message[start : start+end]
}

type apiGenResponseBuffer struct {
	downstream http.ResponseWriter
	request    *http.Request
	header     http.Header
	body       bytes.Buffer
	status     int
}

func newAPIGenResponseBuffer(w http.ResponseWriter, r *http.Request) *apiGenResponseBuffer {
	return &apiGenResponseBuffer{downstream: w, request: r, header: http.Header{}}
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
	body := w.normalizedBody(status)
	if status >= 200 && status < 300 && strings.HasPrefix(w.header.Get("Content-Type"), "application/json") {
		body = signAPIResponseCursor(w.request, body)
	}
	if (status == http.StatusCreated || status == http.StatusAccepted) && w.header.Get("Location") == "" {
		if location := responseLocation(w.request, body); location != "" {
			w.header.Set("Location", location)
		}
	}
	if w.request.Method == http.MethodDelete && status >= 200 && status < 300 {
		status = http.StatusNoContent
		body = nil
		w.header.Del("Content-Type")
		w.header.Del("Content-Length")
	}
	if status == http.StatusOK && w.request.Method == http.MethodGet && strings.HasPrefix(w.header.Get("Content-Type"), "application/json") {
		etag := w.header.Get("ETag")
		if etag == "" {
			etag = strongETag(strings.TrimSpace(string(body)))
			w.header.Set("ETag", etag)
		}
		if etagMatches(w.request.Header.Get("If-None-Match"), etag) {
			status = http.StatusNotModified
			body = nil
			w.header.Del("Content-Type")
			w.header.Del("Content-Length")
		}
	}
	if isQueryRequest(w.request) {
		w.header.Set("Cache-Control", "no-store")
	}
	for key, values := range w.header {
		for _, value := range values {
			w.downstream.Header().Add(key, value)
		}
	}
	w.downstream.WriteHeader(status)
	if len(body) != 0 {
		_, _ = w.downstream.Write(body)
	}
}

func responseLocation(r *http.Request, body []byte) string {
	if r == nil {
		return ""
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	for _, suffix := range []string{"/cancel", "/finalize"} {
		if strings.HasSuffix(path, suffix) {
			return strings.TrimSuffix(path, suffix)
		}
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	id, _ := value["id"].(string)
	if id == "" {
		for _, key := range []string{"principal", "apiToken", "clientSecret"} {
			nested, _ := value[key].(map[string]any)
			if candidate, _ := nested["id"].(string); candidate != "" {
				id = candidate
				break
			}
		}
	}
	if id == "" {
		return ""
	}
	return path + "/" + url.PathEscape(id)
}

func (w *apiGenResponseBuffer) normalizedBody(status int) []byte {
	if status < 400 || w.body.Len() == 0 {
		return w.body.Bytes()
	}
	var value map[string]any
	if err := json.Unmarshal(w.body.Bytes(), &value); err != nil {
		return w.body.Bytes()
	}
	if strings.HasPrefix(w.header.Get("Content-Type"), "application/problem+json") {
		if instance, _ := value["instance"].(string); strings.TrimSpace(instance) == "" {
			value["instance"] = w.request.URL.Path
		}
		if requestID, _ := value["requestId"].(string); strings.TrimSpace(requestID) == "" {
			value["requestId"] = w.request.Header.Get("X-Request-ID")
		}
		if errorsValue, present := value["errors"]; !present || errorsValue == nil {
			value["errors"] = []apigenapi.ProblemFieldError{}
		}
		out, err := json.Marshal(value)
		if err != nil {
			return w.body.Bytes()
		}
		return append(out, '\n')
	}
	if _, ok := value["code"]; !ok {
		return w.body.Bytes()
	}
	message, ok := value["message"].(string)
	if !ok {
		return w.body.Bytes()
	}
	requestID := w.request.Header.Get("X-Request-ID")
	code := fmt.Sprintf("HTTP_%d", status)
	if raw, ok := value["code"].(string); ok && raw != "" {
		code = raw
	}
	errors := []apigenapi.ProblemFieldError{}
	if details, ok := value["details"].(map[string]any); ok {
		if field, ok := details["field"].(string); ok && field != "" {
			errors = append(errors, apigenapi.ProblemFieldError{Field: field, Code: code, Detail: message})
		}
	}
	problem := apigenapi.ProblemDetails{
		Type: "https://leapview.dev/problems/" + strings.ToLower(code), Title: http.StatusText(status), Status: int32(status),
		Detail: message, Instance: w.request.URL.Path, Code: code, RequestId: requestID, Errors: errors,
	}
	w.header.Set("Content-Type", "application/problem+json")
	out, err := json.Marshal(problem)
	if err != nil {
		return w.body.Bytes()
	}
	return append(out, '\n')
}

func etagMatches(raw, current string) bool {
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "*" || value == current {
			return true
		}
	}
	return false
}

func isQueryRequest(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	path := r.URL.Path
	return strings.HasSuffix(path, "/query") || strings.HasSuffix(path, "/query/explain") || strings.HasSuffix(path, "/preview") || strings.HasSuffix(path, "/preview/explain") || strings.HasSuffix(path, "/values") || strings.HasSuffix(path, "/authorization-checks")
}

func (a apiGenAdapter) GetCurrentPrincipal(w http.ResponseWriter, r *http.Request) {
	a.server.accessHTTPHandler().GetCurrentPrincipal(w, r)
}

func (a apiGenAdapter) ListCurrentEffectivePrivileges(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentEffectivePrivilegesParams) {
	a.server.accessHTTPHandler().ListCurrentEffectivePrivileges(w, r)
}

func (a apiGenAdapter) ListCurrentAPITokens(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentAPITokensParams) {
	a.server.accessHTTPHandler().ListCurrentAPITokens(w, r)
}

func (a apiGenAdapter) CreateCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateCurrentAPITokenHeaders) {
	a.server.accessHTTPHandler().CreateCurrentAPIToken(w, r)
}

func (a apiGenAdapter) RevokeCurrentAPIToken(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().RevokeCurrentAPIToken(w, r)
}

func (a apiGenAdapter) ListCurrentSessions(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListCurrentSessionsParams) {
	a.server.accessHTTPHandler().ListCurrentSessions(w, r)
}

func (a apiGenAdapter) GetActiveManagedDataRevision(w http.ResponseWriter, r *http.Request, project, connection string) {
	a.server.managedDataHTTPHandler().GetActiveManagedDataRevision(w, r, project, connection)
}

func (a apiGenAdapter) ListManagedDataRevisions(w http.ResponseWriter, r *http.Request, project, connection string, params apigenapi.GenListManagedDataRevisionsParams) {
	a.server.managedDataHTTPHandler().ListManagedDataRevisions(w, r, project, connection, params)
}

func (a apiGenAdapter) GetManagedDataRevision(w http.ResponseWriter, r *http.Request, project, connection, revision string) {
	a.server.managedDataHTTPHandler().GetManagedDataRevision(w, r, project, connection, revision)
}

func (a apiGenAdapter) CreateManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection string, headers apigenapi.GenCreateManagedDataUploadSessionHeaders) {
	a.server.managedDataHTTPHandler().CreateManagedDataUploadSession(w, r, project, connection, headers)
}

func (a apiGenAdapter) GetManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string) {
	a.server.managedDataHTTPHandler().GetManagedDataUploadSession(w, r, project, connection, uploadSession)
}

func (a apiGenAdapter) ListManagedDataUploadSessions(w http.ResponseWriter, r *http.Request, project, connection string, params apigenapi.GenListManagedDataUploadSessionsParams) {
	a.server.managedDataHTTPHandler().ListManagedDataUploadSessions(w, r, project, connection, params)
}

func (a apiGenAdapter) CancelManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenCancelManagedDataUploadSessionHeaders) {
	a.server.managedDataHTTPHandler().CancelManagedDataUploadSession(w, r, project, connection, uploadSession, headers)
}

func (a apiGenAdapter) FinalizeManagedDataUploadSession(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenFinalizeManagedDataUploadSessionHeaders) {
	a.server.managedDataHTTPHandler().FinalizeManagedDataUploadSession(w, r, project, connection, uploadSession, headers)
}

func (a apiGenAdapter) CreateManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession string, headers apigenapi.GenCreateManagedDataS3MultipartUploadHeaders) {
	a.server.managedDataHTTPHandler().CreateManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, headers)
}

func (a apiGenAdapter) AbortManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, headers apigenapi.GenAbortManagedDataS3MultipartUploadHeaders) {
	a.server.managedDataHTTPHandler().AbortManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, multipartUpload, headers)
}

func (a apiGenAdapter) CompleteManagedDataS3MultipartUpload(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, headers apigenapi.GenCompleteManagedDataS3MultipartUploadHeaders) {
	a.server.managedDataHTTPHandler().CompleteManagedDataS3MultipartUpload(w, r, project, connection, uploadSession, multipartUpload, headers)
}

func (a apiGenAdapter) SignManagedDataS3MultipartPart(w http.ResponseWriter, r *http.Request, project, connection, uploadSession, multipartUpload string, partNumber int32, _ apigenapi.GenSignManagedDataS3MultipartPartHeaders) {
	a.server.managedDataHTTPHandler().SignManagedDataS3MultipartPart(w, r, project, connection, uploadSession, multipartUpload, partNumber)
}

func (a apiGenAdapter) RevokeCurrentSession(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().RevokeCurrentSession(w, r)
}

func (a apiGenAdapter) ListWorkspaces(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListWorkspacesParams) {
	a.server.workspaceHTTPHandler().Workspaces(w, r)
}

func (a apiGenAdapter) Search(w http.ResponseWriter, r *http.Request, params apigenapi.GenSearchParams) {
	a.server.searchAPI(w, r, params)
}

func (a apiGenAdapter) ListWorkspaceAssets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceAssetsParams) {
	a.server.workspaceHTTPHandler().Assets(w, r)
}

func (a apiGenAdapter) GetWorkspaceActiveAssetGraph(w http.ResponseWriter, r *http.Request, _ string) {
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

func (a apiGenAdapter) GetDashboardPage(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.dashboardHTTP().GetDashboardPage(w, r)
}

func (a apiGenAdapter) GetDashboardFilter(w http.ResponseWriter, r *http.Request, _, _, _, _ string) {
	a.server.dashboardHTTP().GetDashboardFilter(w, r)
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

func (a apiGenAdapter) ListSemanticModelFields(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticModelFieldsParams) {
	a.server.semanticQueryHTTP().ListSemanticModelFields(w, r)
}

func (a apiGenAdapter) ListSemanticRelationships(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticRelationshipsParams) {
	a.server.semanticQueryHTTP().ListSemanticRelationships(w, r)
}

func (a apiGenAdapter) ListSemanticSources(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListSemanticSourcesParams) {
	a.server.semanticQueryHTTP().ListSemanticSources(w, r)
}

func (a apiGenAdapter) QuerySemanticModel(w http.ResponseWriter, r *http.Request, workspaceID, _ string, _ apigenapi.GenQuerySemanticModelHeaders) {
	a.setServingSnapshot(r, workspaceID)
	a.server.semanticQueryHTTP().QuerySemanticModel(w, r)
}

func (a apiGenAdapter) ExplainSemanticModelQuery(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.semanticQueryHTTP().ExplainSemanticModelQuery(w, r)
}

func (a apiGenAdapter) ListSemanticFields(w http.ResponseWriter, r *http.Request, _, _, _ string, _ apigenapi.GenListSemanticFieldsParams) {
	a.server.semanticQueryHTTP().ListSemanticFields(w, r)
}

func (a apiGenAdapter) PreviewSemanticDataset(w http.ResponseWriter, r *http.Request, workspaceID, _, _ string, _ apigenapi.GenPreviewSemanticDatasetHeaders) {
	a.setServingSnapshot(r, workspaceID)
	a.server.semanticQueryHTTP().PreviewSemanticDataset(w, r)
}

func (a apiGenAdapter) ExplainSemanticPreview(w http.ResponseWriter, r *http.Request, _, _, _ string) {
	a.server.semanticQueryHTTP().ExplainSemanticPreview(w, r)
}

func (a apiGenAdapter) QueryDashboardPage(w http.ResponseWriter, r *http.Request, workspaceID, _, _ string) {
	a.setServingSnapshot(r, workspaceID)
	a.server.dashboardHTTP().QueryDashboardPage(w, r)
}

func (a apiGenAdapter) QueryDashboardVisualData(w http.ResponseWriter, r *http.Request, workspaceID, _, _, _ string, _ apigenapi.GenQueryDashboardVisualDataHeaders) {
	a.setServingSnapshot(r, workspaceID)
	a.server.dashboardHTTP().QueryDashboardVisualData(w, r)
}

func (a apiGenAdapter) setServingSnapshot(r *http.Request, workspaceID string) {
	r.Header.Del("X-Serving-Snapshot")
	repo, err := a.server.workspaceRepository()
	if err != nil || repo == nil {
		return
	}
	row, err := repo.ByID(r.Context(), workspace.WorkspaceID(a.server.workspaceID(workspaceID)))
	if err == nil && row.ActiveServingStateID != "" {
		r.Header.Set("X-Serving-Snapshot", string(row.ActiveServingStateID))
	}
}

func (a apiGenAdapter) ListDashboardFilterValues(w http.ResponseWriter, r *http.Request, workspaceID, _, _, _ string, _ apigenapi.GenListDashboardFilterValuesParams) {
	a.setServingSnapshot(r, workspaceID)
	a.server.dashboardHTTP().ListDashboardFilterOptions(w, r)
}

func (a apiGenAdapter) CreateRefreshRun(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateRefreshRunHeaders) {
	a.server.refreshRunHTTP().CreateRun(w, r)
}

func (a apiGenAdapter) ListRefreshRuns(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListRefreshRunsParams) {
	a.server.refreshRunHTTP().ListRuns(w, r)
}

func (a apiGenAdapter) GetRefreshRun(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.refreshRunHTTP().GetRun(w, r)
}

func (a apiGenAdapter) ListAgentConversations(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListAgentConversationsParams) {
	a.server.agentHTTPHandler().ListConversations(w, r)
}

func (a apiGenAdapter) CreateAgentConversation(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateAgentConversationHeaders) {
	a.server.agentHTTPHandler().CreateConversation(w, r)
}

func (a apiGenAdapter) GetAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.agentHTTPHandler().GetConversation(w, r)
}

func (a apiGenAdapter) UpdateAgentConversation(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdateAgentConversationHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
	a.server.agentHTTPHandler().UpdateConversation(w, r)
}

func (a apiGenAdapter) ArchiveAgentConversation(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.agentHTTPHandler().ArchiveConversation(w, r)
}

func (a apiGenAdapter) ListAgentMessages(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentMessagesParams) {
	a.server.agentHTTPHandler().ListMessages(w, r)
}

func (a apiGenAdapter) CreateAgentRun(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateAgentRunHeaders) {
	a.server.agentHTTPHandler().CreateRun(w, r)
}

func (a apiGenAdapter) ListAgentRuns(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListAgentRunsParams) {
	a.server.agentHTTPHandler().ListRuns(w, r)
}

func (a apiGenAdapter) GetAgentRun(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.agentHTTPHandler().GetRun(w, r)
}

func (a apiGenAdapter) ListAgentEvents(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenListAgentEventsParams, _ apigenapi.GenListAgentEventsHeaders) {
	a.server.agentHTTPHandler().ListEvents(w, r)
}

func (a apiGenAdapter) CancelAgentRun(w http.ResponseWriter, r *http.Request, _, _ string, _ apigenapi.GenCancelAgentRunHeaders) {
	a.server.agentHTTPHandler().CancelRun(w, r)
}

func (a apiGenAdapter) ListPrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListPrincipalsParams) {
	a.server.accessHTTPHandler().ListPrincipals(w, r)
}

func (a apiGenAdapter) CreatePrincipal(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreatePrincipalHeaders) {
	a.server.accessHTTPHandler().CreatePrincipal(w, r)
}

func (a apiGenAdapter) GetPrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().GetPrincipal(w, r)
}

func (a apiGenAdapter) UpdatePrincipal(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdatePrincipalHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
	a.server.accessHTTPHandler().UpdatePrincipal(w, r)
}

func (a apiGenAdapter) DeletePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().DeletePrincipal(w, r)
}

func (a apiGenAdapter) ResetPrincipalPassword(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenResetPrincipalPasswordHeaders) {
	a.server.accessHTTPHandler().ResetPrincipalPassword(w, r)
}

func (a apiGenAdapter) ListServicePrincipals(w http.ResponseWriter, r *http.Request, _ apigenapi.GenListServicePrincipalsParams) {
	a.server.accessHTTPHandler().ListServicePrincipals(w, r)
}

func (a apiGenAdapter) CreateServicePrincipal(w http.ResponseWriter, r *http.Request, _ apigenapi.GenCreateServicePrincipalHeaders) {
	a.server.accessHTTPHandler().CreateServicePrincipal(w, r)
}

func (a apiGenAdapter) GetServicePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().GetServicePrincipal(w, r)
}

func (a apiGenAdapter) UpdateServicePrincipal(w http.ResponseWriter, r *http.Request, _ string, headers apigenapi.GenUpdateServicePrincipalHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
	a.server.accessHTTPHandler().UpdateServicePrincipal(w, r)
}

func (a apiGenAdapter) DeleteServicePrincipal(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().DeleteServicePrincipal(w, r)
}

func (a apiGenAdapter) CreateServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateServicePrincipalSecretHeaders) {
	a.server.accessHTTPHandler().CreateServicePrincipalSecret(w, r)
}

func (a apiGenAdapter) ListServicePrincipalSecrets(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListServicePrincipalSecretsParams) {
	a.server.accessHTTPHandler().ListServicePrincipalSecrets(w, r)
}

func (a apiGenAdapter) GetServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetServicePrincipalSecret(w, r)
}

func (a apiGenAdapter) RevokeServicePrincipalSecret(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().RevokeServicePrincipalSecret(w, r)
}

func (a apiGenAdapter) ListWorkspaceRoles(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListWorkspaceRolesParams) {
	a.server.accessHTTPHandler().ListWorkspaceRoles(w, r)
}

func (a apiGenAdapter) ListEffectivePrivileges(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListEffectivePrivilegesParams) {
	a.server.accessHTTPHandler().ListEffectivePrivileges(w, r)
}

func (a apiGenAdapter) ListGrants(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListGrantsParams) {
	a.server.accessHTTPHandler().ListGrants(w, r)
}

func (a apiGenAdapter) CreateGrant(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateGrantHeaders) {
	a.server.accessHTTPHandler().CreateGrant(w, r)
}

func (a apiGenAdapter) GetGrant(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetGrant(w, r)
}

func (a apiGenAdapter) UpdateGrant(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateGrantHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
	a.server.accessHTTPHandler().UpdateGrant(w, r)
}

func (a apiGenAdapter) DeleteGrant(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().DeleteGrant(w, r)
}

func (a apiGenAdapter) ListDataPolicies(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenListDataPoliciesParams) {
	a.server.accessHTTPHandler().ListDataPolicies(w, r)
}

func (a apiGenAdapter) CreateDataPolicy(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateDataPolicyHeaders) {
	a.server.accessHTTPHandler().CreateDataPolicy(w, r)
}

func (a apiGenAdapter) GetDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetDataPolicy(w, r)
}

func (a apiGenAdapter) UpdateDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateDataPolicyHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
	a.server.accessHTTPHandler().UpdateDataPolicy(w, r)
}

func (a apiGenAdapter) CheckAuthorizationBatch(w http.ResponseWriter, r *http.Request, _ string) {
	a.server.accessHTTPHandler().CheckAuthorizationBatch(w, r)
}

func (a apiGenAdapter) DeleteDataPolicy(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().DeleteDataPolicy(w, r)
}

func (a apiGenAdapter) TransferOwnership(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenTransferOwnershipHeaders) {
	a.server.accessHTTPHandler().TransferOwnership(w, r)
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

func (a apiGenAdapter) CreateGroup(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateGroupHeaders) {
	a.server.accessHTTPHandler().CreateGroup(w, r)
}

func (a apiGenAdapter) GetGroup(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetGroup(w, r)
}

func (a apiGenAdapter) UpdateGroup(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateGroupHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
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

func (a apiGenAdapter) CreateRoleBinding(w http.ResponseWriter, r *http.Request, _ string, _ apigenapi.GenCreateRoleBindingHeaders) {
	a.server.accessHTTPHandler().CreateRoleBinding(w, r)
}

func (a apiGenAdapter) GetRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string) {
	a.server.accessHTTPHandler().GetRoleBinding(w, r)
}

func (a apiGenAdapter) UpdateRoleBinding(w http.ResponseWriter, r *http.Request, _, _ string, headers apigenapi.GenUpdateRoleBindingHeaders) {
	r.Header.Set("If-Match", headers.IfMatch)
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
