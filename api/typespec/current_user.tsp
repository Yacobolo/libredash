using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
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

alias GetCurrentPrincipalOK = OkJson<PrincipalResponse>;

@tag("Current User")
@summary("Get current principal")
@route("/me")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op getCurrentPrincipal(): GetCurrentPrincipalOK | CommonErrors;

alias ListCurrentAPITokensOK = OkJson<APITokenListResponse>;

@tag("Current User")
@summary("List current API tokens")
@route("/me/api-tokens")
@get
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op listCurrentAPITokens(@query limit?: int32, @query pageToken?: string): ListCurrentAPITokensOK | CommonErrors;

alias CreateCurrentAPITokenCreated = CreatedJson<APITokenCreateResponse>;

@tag("Current User")
@summary("Create current API token")
@route("/me/api-tokens")
@post
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op createCurrentAPIToken(@body body: APITokenCreateRequest): CreateCurrentAPITokenCreated | CommonErrors;

alias RevokeCurrentAPITokenOK = OkJson<StatusResponse>;

@tag("Current User")
@summary("Revoke current API token")
@route("/me/api-tokens/{token}")
@delete
@apigen.authz(#{ mode: "permission", permission: "token:manage" })
op revokeCurrentAPIToken(@path token: string): RevokeCurrentAPITokenOK | CommonErrors;

alias ListCurrentPermissionsOK = OkJson<PermissionListResponse>;

@tag("Current User")
@summary("List current permissions")
@route("/me/permissions")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op listCurrentPermissions(@query workspace?: string): ListCurrentPermissionsOK | CommonErrors;

alias ListCurrentSessionsOK = OkJson<SessionListResponse>;

@tag("Current User")
@summary("List current sessions")
@route("/me/sessions")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op listCurrentSessions(@query limit?: int32, @query pageToken?: string): ListCurrentSessionsOK | CommonErrors;

alias RevokeCurrentSessionOK = OkJson<StatusResponse>;

@tag("Current User")
@summary("Revoke current session")
@route("/me/sessions/{session}")
@delete
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
op revokeCurrentSession(@path session: string): RevokeCurrentSessionOK | CommonErrors;
