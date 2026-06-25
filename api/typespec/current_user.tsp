using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model PermissionListResponse {
  workspaceId: string;
  permissions: string[];
}

model APITokenCreateRequest {
  name: string;
  workspaceId?: string;
  permissions?: string[];
  expiresAt?: utcDateTime;
}

model APITokenResponse {
  id: string;
  name: string;
  workspaceId?: string;
  permissions: string[];
  expiresAt?: utcDateTime;
  revokedAt?: utcDateTime;
  createdAt: utcDateTime;
  lastUsedAt?: utcDateTime;
}

model APITokenCreateResponse {
  token: string;
  apiToken: APITokenResponse;
}

model APITokenListResponse {
  items: APITokenResponse[];
  page: PageInfo;
}

model SessionResponse {
  id: string;
  createdAt: utcDateTime;
  expiresAt: utcDateTime;
  lastSeenAt?: utcDateTime;
}

model SessionListResponse {
  items: SessionResponse[];
  page: PageInfo;
}

model GetCurrentPrincipalOK {
  @statusCode statusCode: 200;
  @body body: PrincipalResponse;
}

@tag("Current User")
@summary("Get current principal")
@route("/api/v1/me")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op getCurrentPrincipal(): GetCurrentPrincipalOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListCurrentAPITokensOK {
  @statusCode statusCode: 200;
  @body body: APITokenListResponse;
}

@tag("Current User")
@summary("List current API tokens")
@route("/api/v1/me/api-tokens")
@get
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op listCurrentAPITokens(@query limit?: int32, @query pageToken?: string): ListCurrentAPITokensOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateCurrentAPITokenCreated {
  @statusCode statusCode: 201;
  @body body: APITokenCreateResponse;
}

@tag("Current User")
@summary("Create current API token")
@route("/api/v1/me/api-tokens")
@post
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op createCurrentAPIToken(@body body: APITokenCreateRequest): CreateCurrentAPITokenCreated | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model RevokeCurrentAPITokenOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Current User")
@summary("Revoke current API token")
@route("/api/v1/me/api-tokens/{token}")
@delete
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op revokeCurrentAPIToken(@path token: string): RevokeCurrentAPITokenOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListCurrentPermissionsOK {
  @statusCode statusCode: 200;
  @body body: PermissionListResponse;
}

@tag("Current User")
@summary("List current permissions")
@route("/api/v1/me/permissions")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op listCurrentPermissions(@query workspace?: string): ListCurrentPermissionsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListCurrentSessionsOK {
  @statusCode statusCode: 200;
  @body body: SessionListResponse;
}

@tag("Current User")
@summary("List current sessions")
@route("/api/v1/me/sessions")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op listCurrentSessions(@query limit?: int32, @query pageToken?: string): ListCurrentSessionsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model RevokeCurrentSessionOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Current User")
@summary("Revoke current session")
@route("/api/v1/me/sessions/{session}")
@delete
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op revokeCurrentSession(@path session: string): RevokeCurrentSessionOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
