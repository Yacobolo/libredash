using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model RefreshRunCreateRequest {
  modelId: string;
  servingStateId?: string;
  targetType?: string;
  targetId?: string;
  triggerType?: string;
  parentRunId?: string;
}

model RefreshRunResponse {
  id: string;
  workspaceId: string;
  modelId: string;
  servingStateId?: string;
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

model RefreshRunListResponse {
  items: RefreshRunResponse[];
  page: PageInfo;
}

alias ListRefreshRunsOK = OkJson<RefreshRunListResponse>;

@tag("Refresh Runs")
@summary("List refresh runs")
@route("/workspaces/{workspace}/refresh-runs")
@get
@apigen.authz(#{ mode: "permission", permission: "refresh:run" })
@extension("x-agent", #{ enabled: true, name: "list_refresh_runs", risk: "read", tags: #["refresh"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "modelId", "status", "createdAt"], cursorPath: "page.nextCursor", count: true } })
op listRefreshRuns(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListRefreshRunsOK | CommonErrors;

alias CreateRefreshRunAccepted = AcceptedJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Create a refresh run")
@route("/workspaces/{workspace}/refresh-runs")
@post
@apigen.authz(#{ mode: "permission", permission: "refresh:run" })
op createRefreshRun(@path workspace: string, @body body: RefreshRunCreateRequest): CreateRefreshRunAccepted | CommonErrors;

alias GetRefreshRunOK = OkJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Get a refresh run")
@route("/workspaces/{workspace}/refresh-runs/{run}")
@get
@apigen.authz(#{ mode: "permission", permission: "refresh:run" })
@extension("x-agent", #{ enabled: true, name: "get_refresh_run", risk: "read", tags: #["refresh"], output: #{ rootFields: #["id", "modelId", "servingStateId", "targetType", "targetId", "status", "error", "createdAt", "startedAt", "finishedAt"] } })
op getRefreshRun(@path workspace: string, @path run: string): GetRefreshRunOK | CommonErrors;
