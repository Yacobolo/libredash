using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model PrincipalResponse {
  id: string;
  email: string;
  displayName: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model PrincipalListResponse {
  items: PrincipalResponse[];
  page: PageInfo;
}

model PrincipalPatchRequest {
  displayName?: string;
}

model RoleResponse {
  name: string;
  permissions: string[];
}

model RoleListResponse {
  items: RoleResponse[];
  page: PageInfo;
}

model GroupCreateRequest {
  name: string;
  displayName?: string;
}

model GroupPatchRequest {
  displayName?: string;
}

model GroupResponse {
  id: string;
  name: string;
  displayName: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model GroupListResponse {
  items: GroupResponse[];
  page: PageInfo;
}

model GroupMemberListResponse {
  items: PrincipalResponse[];
  page: PageInfo;
}

model RoleBindingRequest {
  subjectType: string;
  subjectId: string;
  role: string;
}

model RoleBindingResponse {
  id: string;
  workspaceId: string;
  subjectType: string;
  subjectId: string;
  email?: string;
  displayName?: string;
  role: string;
  createdAt: utcDateTime;
}

model RoleBindingListResponse {
  items: RoleBindingResponse[];
  page: PageInfo;
}

model ListPrincipalsOK {
  @statusCode statusCode: 200;
  @body body: PrincipalListResponse;
}

@tag("Access")
@summary("List principals")
@route("/api/v1/principals")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listPrincipals(@query email?: string, @query q?: string, @query limit?: int32, @query pageToken?: string): ListPrincipalsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetPrincipalOK {
  @statusCode statusCode: 200;
  @body body: PrincipalResponse;
}

@tag("Access")
@summary("Get a principal")
@route("/api/v1/principals/{principal}")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op getPrincipal(@path principal: string): GetPrincipalOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model UpdatePrincipalOK {
  @statusCode statusCode: 200;
  @body body: PrincipalResponse;
}

@tag("Access")
@summary("Update a principal")
@route("/api/v1/principals/{principal}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updatePrincipal(@path principal: string, @body body: PrincipalPatchRequest): UpdatePrincipalOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListGroupsOK {
  @statusCode statusCode: 200;
  @body body: GroupListResponse;
}

@tag("Access")
@summary("List groups")
@route("/api/v1/workspaces/{workspace}/groups")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listGroups(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListGroupsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateGroupCreated {
  @statusCode statusCode: 201;
  @body body: GroupResponse;
}

@tag("Access")
@summary("Create a group")
@route("/api/v1/workspaces/{workspace}/groups")
@post
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op createGroup(@path workspace: string, @body body: GroupCreateRequest): CreateGroupCreated | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model DeleteGroupOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Access")
@summary("Delete a group")
@route("/api/v1/workspaces/{workspace}/groups/{group}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op deleteGroup(@path workspace: string, @path group: string): DeleteGroupOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetGroupOK {
  @statusCode statusCode: 200;
  @body body: GroupResponse;
}

@tag("Access")
@summary("Get a group")
@route("/api/v1/workspaces/{workspace}/groups/{group}")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op getGroup(@path workspace: string, @path group: string): GetGroupOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model UpdateGroupOK {
  @statusCode statusCode: 200;
  @body body: GroupResponse;
}

@tag("Access")
@summary("Update a group")
@route("/api/v1/workspaces/{workspace}/groups/{group}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updateGroup(@path workspace: string, @path group: string, @body body: GroupPatchRequest): UpdateGroupOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListGroupMembersOK {
  @statusCode statusCode: 200;
  @body body: GroupMemberListResponse;
}

@tag("Access")
@summary("List group members")
@route("/api/v1/workspaces/{workspace}/groups/{group}/members")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listGroupMembers(@path workspace: string, @path group: string, @query limit?: int32, @query pageToken?: string): ListGroupMembersOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model RemoveGroupMemberOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Access")
@summary("Remove a group member")
@route("/api/v1/workspaces/{workspace}/groups/{group}/members/{principal}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op removeGroupMember(@path workspace: string, @path group: string, @path principal: string): RemoveGroupMemberOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model AddGroupMemberOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Access")
@summary("Add a group member")
@route("/api/v1/workspaces/{workspace}/groups/{group}/members/{principal}")
@put
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op addGroupMember(@path workspace: string, @path group: string, @path principal: string): AddGroupMemberOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListRoleBindingsOK {
  @statusCode statusCode: 200;
  @body body: RoleBindingListResponse;
}

@tag("Access")
@summary("List role bindings")
@route("/api/v1/workspaces/{workspace}/role-bindings")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listRoleBindings(@path workspace: string, @query subjectType?: string, @query subjectId?: string, @query role?: string, @query limit?: int32, @query pageToken?: string): ListRoleBindingsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateRoleBindingCreated {
  @statusCode statusCode: 201;
  @body body: RoleBindingResponse;
}

@tag("Access")
@summary("Create a role binding")
@route("/api/v1/workspaces/{workspace}/role-bindings")
@post
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op createRoleBinding(@path workspace: string, @body body: RoleBindingRequest): CreateRoleBindingCreated | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model DeleteRoleBindingOK {
  @statusCode statusCode: 200;
  @body body: StatusResponse;
}

@tag("Access")
@summary("Delete a role binding")
@route("/api/v1/workspaces/{workspace}/role-bindings/{binding}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op deleteRoleBinding(@path workspace: string, @path binding: string): DeleteRoleBindingOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model UpdateRoleBindingOK {
  @statusCode statusCode: 200;
  @body body: RoleBindingResponse;
}

@tag("Access")
@summary("Update a role binding")
@route("/api/v1/workspaces/{workspace}/role-bindings/{binding}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updateRoleBinding(@path workspace: string, @path binding: string, @body body: RoleBindingRequest): UpdateRoleBindingOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListWorkspaceRolesOK {
  @statusCode statusCode: 200;
  @body body: RoleListResponse;
}

@tag("Access")
@summary("List workspace roles")
@route("/api/v1/workspaces/{workspace}/roles")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listWorkspaceRoles(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceRolesOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
