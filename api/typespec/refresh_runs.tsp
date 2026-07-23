using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

model RefreshRunCreateRequest {
  pipelineId: string;
  retryOf?: string;
}

model RefreshRunResponse {
  id: string;
  workspaceId: string;
  pipelineId: string;
  semanticModel: string;
  principalId?: string;
  principalDisplayName?: string;
  trigger: string;
  retryOf?: string;
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
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op listRefreshRuns(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListRefreshRunsOK | CommonErrors;

alias CreateRefreshRunAccepted = AcceptedJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Create a refresh run")
@route("/workspaces/{workspace}/refresh-runs")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op createRefreshRun(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: RefreshRunCreateRequest): CreateRefreshRunAccepted | CommonErrors;

alias GetRefreshRunOK = OkJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Get a refresh run")
@route("/workspaces/{workspace}/refresh-runs/{run}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op getRefreshRun(@path workspace: string, @path run: string): GetRefreshRunOK | CommonErrors;

@tag("Refresh Runs")
@summary("List or stream refresh run events")
@route("/workspaces/{workspace}/refresh-runs/{run}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op listRefreshRunEvents(@path workspace: string, @path run: string, @header accept?: string, @header("Last-Event-ID") lastEventId?: string, @query limit?: ListLimit, @query pageToken?: PageToken): EventHistoryResponse<AsyncEventListResponse> | CommonErrors;

@tag("Refresh Runs")
@summary("Cancel a refresh run")
@route("/workspaces/{workspace}/refresh-runs/{run}/cancel")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op cancelRefreshRun(@path workspace: string, @path run: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<RefreshRunResponse> | CommonErrors;
