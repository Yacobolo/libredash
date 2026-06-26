using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model MaterializationRunCreateRequest {
  modelId: string;
  deploymentId?: string;
  targetType?: string;
  targetId?: string;
  triggerType?: string;
  parentRunId?: string;
}

model MaterializationRunResponse {
  id: string;
  workspaceId: string;
  modelId: string;
  deploymentId?: string;
  principalId?: string;
  principalDisplayName?: string;
  targetType: string;
  targetId: string;
  triggerType: string;
  parentRunId?: string;
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

alias ListMaterializationRunsOK = OkJson<MaterializationRunListResponse>;

@tag("Materializations")
@summary("List materialization runs")
@route("/workspaces/{workspace}/materialization-runs")
@get
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
@extension("x-agent", #{ enabled: true, name: "list_materialization_runs", risk: "read", tags: #["materialization"], defaultLimit: 25 })
op listMaterializationRuns(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListMaterializationRunsOK | CommonErrors;

alias CreateMaterializationRunAccepted = AcceptedJson<MaterializationRunResponse>;

@tag("Materializations")
@summary("Create a materialization run")
@route("/workspaces/{workspace}/materialization-runs")
@post
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
op createMaterializationRun(@path workspace: string, @body body: MaterializationRunCreateRequest): CreateMaterializationRunAccepted | CommonErrors;

alias GetMaterializationRunOK = OkJson<MaterializationRunResponse>;

@tag("Materializations")
@summary("Get a materialization run")
@route("/workspaces/{workspace}/materialization-runs/{run}")
@get
@apigen.authz(#{ mode: "permission", permission: "materialization:run" })
@extension("x-agent", #{ enabled: true, name: "get_materialization_run", risk: "read", tags: #["materialization"] })
op getMaterializationRun(@path workspace: string, @path run: string): GetMaterializationRunOK | CommonErrors;
