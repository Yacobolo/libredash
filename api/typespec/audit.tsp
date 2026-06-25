using Http;
using TypeSpec.OpenAPI;

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

model ListAuditEventsOK {
  @statusCode statusCode: 200;
  @body body: AuditEventListResponse;
}

@tag("Audit")
@summary("List workspace audit events")
@route("/api/v1/workspaces/{workspace}/audit-events")
@get
@apigen.authz(#{ mode: "permission", permission: "audit:read" })
op listAuditEvents(@path workspace: string, @query actor?: string, @query action?: string, @query targetType?: string, @query targetId?: string, @query from?: utcDateTime, @query to?: utcDateTime, @query limit?: int32, @query pageToken?: string): ListAuditEventsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
