using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model AuditEventResponse {
  id: string;
  workspaceId: string;
  principalId?: string;
  action: string;
  targetType: string;
  targetId: string;
  metadata: Record<string>;
  createdAt: utcDateTime;
}

model AuditEventListResponse {
  items: AuditEventResponse[];
  page: PageInfo;
}

model QueryEventResponse {
  id: string;
  workspaceId: string;
  principalId?: string;
  surface: string;
  operation: string;
  queryKind: string;
  modelId: string;
  target: string;
  objectType: string;
  objectId: string;
  requestId: string;
  correlationId: string;
  status: string;
  durationMs: int64;
  rowsReturned: int32;
  bytesEstimate: int64;
  error?: string;
  sql?: string;
  planText?: string;
  query: Record<string>;
  createdAt: utcDateTime;
}

model QueryEventListResponse {
  items: QueryEventResponse[];
  page: PageInfo;
}

alias ListAuditEventsOK = OkJson<AuditEventListResponse>;
alias ListQueryEventsOK = OkJson<QueryEventListResponse>;

@tag("Audit")
@summary("List workspace audit events")
@route("/workspaces/{workspace}/audit-events")
@get
@apigen.authz(#{ mode: "permission", permission: "audit:read" })
op listAuditEvents(@path workspace: string, @query actor?: string, @query action?: string, @query targetType?: string, @query targetId?: string, @query from?: utcDateTime, @query to?: utcDateTime, @query limit?: int32, @query pageToken?: string): ListAuditEventsOK | CommonErrors;

@tag("Audit")
@summary("List workspace query audit events")
@route("/workspaces/{workspace}/query-events")
@get
@apigen.authz(#{ mode: "permission", permission: "audit:read" })
op listQueryEvents(@path workspace: string, @query principal?: string, @query surface?: string, @query operation?: string, @query kind?: string, @query modelId?: string, @query target?: string, @query status?: string, @query search?: string, @query from?: utcDateTime, @query to?: utcDateTime, @query limit?: int32, @query pageToken?: string): ListQueryEventsOK | CommonErrors;
