using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model RefreshRunCreateRequest {
  modelId?: string;
  servingStateId?: string;
  targetType?: string;
  targetId?: string;
  triggerType?: string;
  parentRunId?: string;
  retryOf?: string;
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
@apigen.authz(#{ mode: "privilege", privilege: "REFRESH_DATA" })
@apigen.tool(#{
  name: "list_refresh_runs",
  effect: "read",
  tags: #["refresh"],
  input: #{ fields: #[
    #{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" },
    #{ source: "query", name: "limit", default: 25 },
  ] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/id" }, #{ source: "/modelId" }, #{ source: "/status" }, #{ source: "/createdAt" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op listRefreshRuns(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListRefreshRunsOK | CommonErrors;

alias CreateRefreshRunAccepted = AcceptedJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Create a refresh run")
@route("/workspaces/{workspace}/refresh-runs")
@post
@apigen.authz(#{ mode: "privilege", privilege: "REFRESH_DATA" })
op createRefreshRun(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: RefreshRunCreateRequest): CreateRefreshRunAccepted | CommonErrors;

alias GetRefreshRunOK = OkJson<RefreshRunResponse>;

@tag("Refresh Runs")
@summary("Get a refresh run")
@route("/workspaces/{workspace}/refresh-runs/{run}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "REFRESH_DATA" })
@apigen.tool(#{
  name: "get_refresh_run",
  effect: "read",
  tags: #["refresh"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" }] },
  output: #{ mode: "project", select: #[#{ source: "/id" }, #{ source: "/modelId" }, #{ source: "/servingStateId" }, #{ source: "/targetType" }, #{ source: "/targetId" }, #{ source: "/status" }, #{ source: "/error" }, #{ source: "/createdAt" }, #{ source: "/startedAt" }, #{ source: "/finishedAt" }] },
})
op getRefreshRun(@path workspace: string, @path run: string): GetRefreshRunOK | CommonErrors;

@tag("Refresh Runs")
@summary("List or stream refresh run events")
@route("/workspaces/{workspace}/refresh-runs/{run}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "REFRESH_DATA" })
op listRefreshRunEvents(@path workspace: string, @path run: string, @header accept?: string, @header("Last-Event-ID") lastEventId?: string, @query limit?: ListLimit, @query pageToken?: PageToken): EventHistoryResponse<AsyncEventListResponse> | CommonErrors;

@tag("Refresh Runs")
@summary("Cancel a refresh run")
@route("/workspaces/{workspace}/refresh-runs/{run}/cancel")
@post
@apigen.authz(#{ mode: "privilege", privilege: "REFRESH_DATA" })
op cancelRefreshRun(@path workspace: string, @path run: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<RefreshRunResponse> | CommonErrors;
