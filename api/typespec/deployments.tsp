using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

@minLength(1)
@maxLength(160)
scalar ProjectDeploymentId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ProjectDeploymentProjectId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ProjectDeploymentEnvironmentId extends string;

@minLength(1)
@maxLength(128)
@pattern("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
scalar ProjectDeploymentWorkspaceId extends string;

@minLength(1)
@maxLength(160)
scalar DeploymentCandidateId extends string;

@minLength(1)
@maxLength(255)
scalar ProjectDeploymentIdempotencyKey extends string;

@pattern("^sha256:[0-9a-f]{64}$")
@minLength(71)
@maxLength(71)
scalar ProjectDeploymentRequestDigest extends string;

enum ProjectDeploymentStatus {
  pending;
  active;
  failed;
  superseded;
}

enum ProjectDeploymentTargetStatus {
  pending;
  active;
  failed;
}

model ProjectDeploymentTargetRequest {
  workspace: ProjectDeploymentWorkspaceId;
  candidateId: DeploymentCandidateId;
}

model ProjectDeploymentCreateRequest {
  environment: ProjectDeploymentEnvironmentId;
  @minItems(1)
  @maxItems(1000)
  targets: ProjectDeploymentTargetRequest[];
}

model ProjectDeploymentTargetResponse {
  workspace: ProjectDeploymentWorkspaceId;
  candidateId: DeploymentCandidateId;
  priorCandidateId?: DeploymentCandidateId;
  status: ProjectDeploymentTargetStatus;
  activatedAt?: utcDateTime;
  error?: string;
}

model ProjectDeploymentConnectionResponse {
  connection: string;
  @pattern("^sha256:[0-9a-f]{64}$")
  revisionId: string;
  @pattern("^sha256:[0-9a-f]{64}$")
  priorRevisionId?: string;
  priorGeneration: int64;
  activatedGeneration?: int64;
}

model ProjectDeploymentResponse {
  id: ProjectDeploymentId;
  project: ProjectDeploymentProjectId;
  environment: ProjectDeploymentEnvironmentId;
  requestDigest: ProjectDeploymentRequestDigest;
  status: ProjectDeploymentStatus;
  @minItems(1)
  @maxItems(1000)
  targets: ProjectDeploymentTargetResponse[];
  @maxItems(1000)
  connections: ProjectDeploymentConnectionResponse[];
  createdAt: utcDateTime;
  activatedAt?: utcDateTime;
  error?: string;
}

@tag("Deployments")
@summary("Create an atomic project deployment")
@route("/projects/{project}/deployments")
@post
@operationId("createProjectDeployment")
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
@apigen.cli(#{ command: #["deployments", "create"], args: #[#{ source: "path", name: "project", displayName: "project" }], output: #{ mode: "raw" } })
op createProjectDeployment(
  @path project: ProjectDeploymentProjectId,
  @header("Idempotency-Key") idempotencyKey: ProjectDeploymentIdempotencyKey,
  @body body: ProjectDeploymentCreateRequest,
): CreatedJson<ProjectDeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Get an atomic project deployment")
@route("/projects/{project}/deployments/{deployment}")
@get
@operationId("getProjectDeployment")
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["deployments", "get"], args: #[#{ source: "path", name: "project", displayName: "project" }, #{ source: "path", name: "deployment", displayName: "deployment" }], output: #{ mode: "raw" } })
op getProjectDeployment(
  @path project: ProjectDeploymentProjectId,
  @path deployment: ProjectDeploymentId,
): OkJson<ProjectDeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Atomically activate a project deployment")
@route("/projects/{project}/deployments/{deployment}/activate")
@post
@operationId("activateProjectDeployment")
@apigen.authz(#{ mode: "privilege", privilege: "ACTIVATE_DEPLOYMENT" })
@apigen.cli(#{ command: #["deployments", "activate"], args: #[#{ source: "path", name: "project", displayName: "project" }, #{ source: "path", name: "deployment", displayName: "deployment" }], output: #{ mode: "raw" } })
op activateProjectDeployment(
  @path project: ProjectDeploymentProjectId,
  @path deployment: ProjectDeploymentId,
  @header("Idempotency-Key") idempotencyKey: ProjectDeploymentIdempotencyKey,
): OkJson<ProjectDeploymentResponse> | CommonErrors;
