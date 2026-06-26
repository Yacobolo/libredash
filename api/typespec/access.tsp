using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
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

alias ListPrincipalsOK = OkJson<PrincipalListResponse>;

@tag("Access")
@summary("List principals")
@route("/principals")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listPrincipals(@query email?: string, @query q?: string, @query limit?: int32, @query pageToken?: string): ListPrincipalsOK | CommonErrors;

alias GetPrincipalOK = OkJson<PrincipalResponse>;

@tag("Access")
@summary("Get a principal")
@route("/principals/{principal}")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op getPrincipal(@path principal: string): GetPrincipalOK | CommonErrors;

alias UpdatePrincipalOK = OkJson<PrincipalResponse>;

@tag("Access")
@summary("Update a principal")
@route("/principals/{principal}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updatePrincipal(@path principal: string, @body body: PrincipalPatchRequest): UpdatePrincipalOK | CommonErrors;

alias ListGroupsOK = OkJson<GroupListResponse>;

@tag("Access")
@summary("List groups")
@route("/workspaces/{workspace}/groups")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listGroups(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListGroupsOK | CommonErrors;

alias CreateGroupCreated = CreatedJson<GroupResponse>;

@tag("Access")
@summary("Create a group")
@route("/workspaces/{workspace}/groups")
@post
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op createGroup(@path workspace: string, @body body: GroupCreateRequest): CreateGroupCreated | CommonErrors;

alias DeleteGroupOK = OkJson<StatusResponse>;

@tag("Access")
@summary("Delete a group")
@route("/workspaces/{workspace}/groups/{group}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op deleteGroup(@path workspace: string, @path group: string): DeleteGroupOK | CommonErrors;

alias GetGroupOK = OkJson<GroupResponse>;

@tag("Access")
@summary("Get a group")
@route("/workspaces/{workspace}/groups/{group}")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op getGroup(@path workspace: string, @path group: string): GetGroupOK | CommonErrors;

alias UpdateGroupOK = OkJson<GroupResponse>;

@tag("Access")
@summary("Update a group")
@route("/workspaces/{workspace}/groups/{group}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updateGroup(@path workspace: string, @path group: string, @body body: GroupPatchRequest): UpdateGroupOK | CommonErrors;

alias ListGroupMembersOK = OkJson<GroupMemberListResponse>;

@tag("Access")
@summary("List group members")
@route("/workspaces/{workspace}/groups/{group}/members")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listGroupMembers(@path workspace: string, @path group: string, @query limit?: int32, @query pageToken?: string): ListGroupMembersOK | CommonErrors;

alias RemoveGroupMemberOK = OkJson<StatusResponse>;

@tag("Access")
@summary("Remove a group member")
@route("/workspaces/{workspace}/groups/{group}/members/{principal}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op removeGroupMember(@path workspace: string, @path group: string, @path principal: string): RemoveGroupMemberOK | CommonErrors;

alias AddGroupMemberOK = OkJson<StatusResponse>;

@tag("Access")
@summary("Add a group member")
@route("/workspaces/{workspace}/groups/{group}/members/{principal}")
@put
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op addGroupMember(@path workspace: string, @path group: string, @path principal: string): AddGroupMemberOK | CommonErrors;

alias ListRoleBindingsOK = OkJson<RoleBindingListResponse>;

@tag("Access")
@summary("List role bindings")
@route("/workspaces/{workspace}/role-bindings")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listRoleBindings(@path workspace: string, @query subjectType?: string, @query subjectId?: string, @query role?: string, @query limit?: int32, @query pageToken?: string): ListRoleBindingsOK | CommonErrors;

alias CreateRoleBindingCreated = CreatedJson<RoleBindingResponse>;

@tag("Access")
@summary("Create a role binding")
@route("/workspaces/{workspace}/role-bindings")
@post
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op createRoleBinding(@path workspace: string, @body body: RoleBindingRequest): CreateRoleBindingCreated | CommonErrors;

alias DeleteRoleBindingOK = OkJson<StatusResponse>;

@tag("Access")
@summary("Delete a role binding")
@route("/workspaces/{workspace}/role-bindings/{binding}")
@delete
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op deleteRoleBinding(@path workspace: string, @path binding: string): DeleteRoleBindingOK | CommonErrors;

alias UpdateRoleBindingOK = OkJson<RoleBindingResponse>;

@tag("Access")
@summary("Update a role binding")
@route("/workspaces/{workspace}/role-bindings/{binding}")
@patch
@apigen.authz(#{ mode: "permission", permission: "rbac:write" })
op updateRoleBinding(@path workspace: string, @path binding: string, @body body: RoleBindingRequest): UpdateRoleBindingOK | CommonErrors;

alias ListWorkspaceRolesOK = OkJson<RoleListResponse>;

@tag("Access")
@summary("List workspace roles")
@route("/workspaces/{workspace}/roles")
@get
@apigen.authz(#{ mode: "permission", permission: "rbac:read" })
op listWorkspaceRoles(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceRolesOK | CommonErrors;
