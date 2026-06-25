using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model MaterializationRunCreateRequest {
  modelId: string;
  deploymentId?: string;
}

model MaterializationRunResponse {
  id: string;
  workspaceId: string;
  modelId: string;
  deploymentId?: string;
  status: string;
  error?: string;
  createdAt: utcDateTime;
  startedAt?: utcDateTime;
  finishedAt?: utcDateTime;
}

model MaterializationRunListResponse {
  items: MaterializationRunResponse[];
  page: PageInfo;
}

model ListMaterializationRunsOK {
  @statusCode statusCode: 200;
  @body body: MaterializationRunListResponse;
}

@tag("Materializations")
@summary("List materialization runs")
@route("/api/v1/workspaces/{workspace}/materialization-runs")
@get
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
op listMaterializationRuns(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListMaterializationRunsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateMaterializationRunAccepted {
  @statusCode statusCode: 202;
  @body body: MaterializationRunResponse;
}

@tag("Materializations")
@summary("Create a materialization run")
@route("/api/v1/workspaces/{workspace}/materialization-runs")
@post
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
op createMaterializationRun(@path workspace: string, @body body: MaterializationRunCreateRequest): CreateMaterializationRunAccepted | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetMaterializationRunOK {
  @statusCode statusCode: 200;
  @body body: MaterializationRunResponse;
}

@tag("Materializations")
@summary("Get a materialization run")
@route("/api/v1/workspaces/{workspace}/materialization-runs/{run}")
@get
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
op getMaterializationRun(@path workspace: string, @path run: string): GetMaterializationRunOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
