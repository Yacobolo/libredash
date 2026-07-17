using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

enum DeploymentStatus {
  queued;
  running;
  active;
  failed;
  cancelled;
  superseded;
}

model DeploymentCreateRequest {
  releaseId: string;
}

model DeploymentTargetResponse {
  workspaceId: string;
  servingStateId?: string;
  priorServingStateId?: string;
  status: string;
  error?: string;
}

model DeploymentConnectionResponse {
  connectionId: string;
  revisionId: string;
  priorRevisionId?: string;
}

model DeploymentProgress {
  current: int32;
  total: int32;
  unit: string;
}

model DeploymentResponse {
  id: string;
  projectId: string;
  releaseId: string;
  environment: string;
  status: DeploymentStatus;
  progress?: DeploymentProgress;
  targets: DeploymentTargetResponse[];
  connections: DeploymentConnectionResponse[];
  createdBy: string;
  createdAt: utcDateTime;
  startedAt?: utcDateTime;
  finishedAt?: utcDateTime;
  error?: string;
}

model DeploymentListResponse {
  items: DeploymentResponse[];
  page: PageInfo;
}

@tag("Deployments")
@summary("List project deployments")
@route("/projects/{project}/deployments")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
op listDeployments(@path project: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<DeploymentListResponse> | CommonErrors;

@tag("Deployments")
@summary("Deploy a ready project release")
@route("/projects/{project}/deployments")
@post
@apigen.authz(#{ mode: "privilege", privilege: "ACTIVATE_DEPLOYMENT" })
op createDeployment(@path project: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: DeploymentCreateRequest): AcceptedJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Get a project deployment")
@route("/projects/{project}/deployments/{deployment}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
op getDeployment(@path project: string, @path deployment: string): OkJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("List or stream deployment events")
@route("/projects/{project}/deployments/{deployment}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
op listDeploymentEvents(@path project: string, @path deployment: string, @header accept?: string, @header("Last-Event-ID") lastEventId?: string, @query limit?: ListLimit, @query pageToken?: PageToken): EventHistoryResponse<AsyncEventListResponse> | CommonErrors;

@tag("Deployments")
@summary("Cancel a deployment before cutover")
@route("/projects/{project}/deployments/{deployment}/cancel")
@post
@apigen.authz(#{ mode: "privilege", privilege: "ACTIVATE_DEPLOYMENT" })
op cancelDeployment(@path project: string, @path deployment: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Roll back by creating a deployment of the prior release")
@route("/projects/{project}/deployments/{deployment}/rollback")
@post
@apigen.authz(#{ mode: "privilege", privilege: "ACTIVATE_DEPLOYMENT" })
op rollbackDeployment(@path project: string, @path deployment: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<DeploymentResponse> | CommonErrors;
