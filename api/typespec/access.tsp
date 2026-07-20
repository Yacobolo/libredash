using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

model PrincipalResponse {
  id: string;
  kind: string;
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

model PrincipalCreateRequest {
  email: string;
  displayName?: string;
}

model LocalPasswordResetResponse {
  principal: PrincipalResponse;
  temporaryPassword: string;
}

model ServicePrincipalCreateRequest {
  id?: string;
  displayName: string;
}

model ServicePrincipalPatchRequest {
  displayName?: string;
}

model ServicePrincipalListResponse {
  items: PrincipalResponse[];
  page: PageInfo;
}

model ServicePrincipalSecretCreateRequest {
  name: string;
  expiresAt?: utcDateTime;
}

model ServicePrincipalSecretResponse {
  id: string;
  servicePrincipalId: string;
  name: string;
  secret?: string;
  expiresAt?: utcDateTime;
  createdAt?: utcDateTime;
  revokedAt?: utcDateTime;
}

model ServicePrincipalSecretCreatedResponse {
  secret: string;
  clientSecret: ServicePrincipalSecretResponse;
}

model ServicePrincipalSecretListResponse {
  items: ServicePrincipalSecretResponse[];
  page: PageInfo;
}

model RoleResponse {
  name: string;
  privileges: string[];
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

model GrantRequest {
  objectType?: string;
  objectId?: string;
  subjectType: string;
  subjectId: string;
  privilege: string;
}

model GrantResponse {
  id: string;
  objectId: string;
  objectType: string;
  workspaceId: string;
  subjectType: string;
  subjectId: string;
  privilege: string;
  inherited?: boolean;
  parentId?: string;
  parentType?: string;
  parentObject?: string;
  createdAt: utcDateTime;
}

model GrantListResponse {
  items: GrantResponse[];
  page: PageInfo;
}

model DataPolicyRequest {
  objectType?: string;
  objectId?: string;
  subjectType?: string;
  subjectId?: string;
  policyType: string;
  expression: JSONObject;
}

model DataPolicyResponse {
  id: string;
  workspaceId: string;
  objectId: string;
  subjectType?: string;
  subjectId?: string;
  policyType: string;
  expression: JSONObject;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model DataPolicyListResponse {
  items: DataPolicyResponse[];
  page: PageInfo;
}

model EffectivePrivilegeListResponse {
  workspaceId: string;
  objectType: string;
  objectId?: string;
  privileges: string[];
  effectiveGrants: AuthorizationDecisionResponse[];
}

model OwnershipTransferRequest {
  objectType?: string;
  objectId?: string;
  ownerPrincipalId: string;
}

model SecurableObjectResponse {
  id: string;
  type: string;
  workspaceId: string;
  parentId?: string;
  ownerPrincipalId?: string;
  displayName?: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model AuthorizationDecisionResponse {

  allowed?: boolean;
  privilege: string;
  reason: string;
  objectType: string;
  objectId?: string;
  grantId?: string;
  grantObjectId?: string;
  subjectType?: string;
  subjectId?: string;
  inherited: boolean;
  owner: boolean;
  platform: boolean;
}

model AuthorizationCheckRequest {
  privilege: string;
  objectType?: string;
  objectId?: string;
}

model AuthorizationBatchCheckRequest {
  checks: AuthorizationCheckRequest[];
}

model AuthorizationBatchCheckResponse {
  decisions: AuthorizationDecisionResponse[];
}

alias ListPrincipalsOK = OkJson<PrincipalListResponse>;

@tag("Access")
@summary("List principals")
@route("/principals")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listPrincipals(@query email?: string, @query q?: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListPrincipalsOK | CommonErrors;

alias CreatePrincipalCreated = CreatedJson<LocalPasswordResetResponse>;

@tag("Access")
@summary("Create a local principal")
@route("/principals")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op createPrincipal(@header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: PrincipalCreateRequest): CreatePrincipalCreated | CommonErrors;

alias GetPrincipalOK = OkJson<PrincipalResponse>;

@tag("Access")
@summary("Get a principal")
@route("/principals/{principal}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getPrincipal(@path principal: string): GetPrincipalOK | CommonErrors;

alias UpdatePrincipalOK = OkJson<PrincipalResponse>;

@tag("Access")
@summary("Update a principal")
@route("/principals/{principal}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updatePrincipal(@path principal: string, @header("If-Match") ifMatch: string, @body body: PrincipalPatchRequest): UpdatePrincipalOK | CommonErrors;

@tag("Access")
@summary("Delete a principal")
@route("/principals/{principal}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op deletePrincipal(@path principal: string): NoContentResponse | CommonErrors;

alias ResetPrincipalPasswordOK = OkJson<LocalPasswordResetResponse>;

@tag("Access")
@summary("Reset a local principal password")
@route("/principals/{principal}/password-reset")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op resetPrincipalPassword(@path principal: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): ResetPrincipalPasswordOK | CommonErrors;

alias ListServicePrincipalsOK = OkJson<ServicePrincipalListResponse>;

@tag("Access")
@summary("List service principals")
@route("/service-principals")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op listServicePrincipals(@query limit?: ListLimit, @query pageToken?: PageToken): ListServicePrincipalsOK | CommonErrors;

alias CreateServicePrincipalCreated = CreatedJson<PrincipalResponse>;

@tag("Access")
@summary("Create a service principal")
@route("/service-principals")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op createServicePrincipal(@header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: ServicePrincipalCreateRequest): CreateServicePrincipalCreated | CommonErrors;

@tag("Access")
@summary("Get a service principal")
@route("/service-principals/{servicePrincipal}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op getServicePrincipal(@path servicePrincipal: string): OkJson<PrincipalResponse> | CommonErrors;

alias UpdateServicePrincipalOK = OkJson<PrincipalResponse>;

@tag("Access")
@summary("Update a service principal")
@route("/service-principals/{servicePrincipal}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op updateServicePrincipal(@path servicePrincipal: string, @header("If-Match") ifMatch: string, @body body: ServicePrincipalPatchRequest): UpdateServicePrincipalOK | CommonErrors;

@tag("Access")
@summary("Delete a service principal")
@route("/service-principals/{servicePrincipal}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op deleteServicePrincipal(@path servicePrincipal: string): NoContentResponse | CommonErrors;

alias CreateServicePrincipalSecretCreated = CreatedJson<ServicePrincipalSecretCreatedResponse>;

@tag("Access")
@summary("Create a service principal secret")
@route("/service-principals/{servicePrincipal}/secrets")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op createServicePrincipalSecret(@path servicePrincipal: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: ServicePrincipalSecretCreateRequest): CreateServicePrincipalSecretCreated | CommonErrors;

@tag("Access")
@summary("List service-principal secrets")
@route("/service-principals/{servicePrincipal}/secrets")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op listServicePrincipalSecrets(@path servicePrincipal: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<ServicePrincipalSecretListResponse> | CommonErrors;

@tag("Access")
@summary("Get service-principal secret metadata")
@route("/service-principals/{servicePrincipal}/secrets/{secret}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op getServicePrincipalSecret(@path servicePrincipal: string, @path secret: string): OkJson<ServicePrincipalSecretResponse> | CommonErrors;

@tag("Access")
@summary("Revoke a service principal secret")
@route("/service-principals/{servicePrincipal}/secrets/{secret}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_PLATFORM" })
op revokeServicePrincipalSecret(@path servicePrincipal: string, @path secret: string): NoContentResponse | CommonErrors;

alias ListGroupsOK = OkJson<GroupListResponse>;

@tag("Access")
@summary("List groups")
@route("/workspaces/{workspace}/groups")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listGroups(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListGroupsOK | CommonErrors;

alias CreateGroupCreated = CreatedJson<GroupResponse>;

@tag("Access")
@summary("Create a group")
@route("/workspaces/{workspace}/groups")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op createGroup(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: GroupCreateRequest): CreateGroupCreated | CommonErrors;

@tag("Access")
@summary("Delete a group")
@route("/workspaces/{workspace}/groups/{group}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op deleteGroup(@path workspace: string, @path group: string): NoContentResponse | CommonErrors;

alias GetGroupOK = OkJson<GroupResponse>;

@tag("Access")
@summary("Get a group")
@route("/workspaces/{workspace}/groups/{group}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getGroup(@path workspace: string, @path group: string): GetGroupOK | CommonErrors;

alias UpdateGroupOK = OkJson<GroupResponse>;

@tag("Access")
@summary("Update a group")
@route("/workspaces/{workspace}/groups/{group}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updateGroup(@path workspace: string, @path group: string, @header("If-Match") ifMatch: string, @body body: GroupPatchRequest): UpdateGroupOK | CommonErrors;

alias ListGroupMembersOK = OkJson<GroupMemberListResponse>;

@tag("Access")
@summary("List group members")
@route("/workspaces/{workspace}/groups/{group}/members")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listGroupMembers(@path workspace: string, @path group: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListGroupMembersOK | CommonErrors;

@tag("Access")
@summary("Remove a group member")
@route("/workspaces/{workspace}/groups/{group}/members/{principal}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op removeGroupMember(@path workspace: string, @path group: string, @path principal: string): NoContentResponse | CommonErrors;

alias AddGroupMemberOK = OkJson<StatusResponse>;

@tag("Access")
@summary("Add a group member")
@route("/workspaces/{workspace}/groups/{group}/members/{principal}")
@put
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op addGroupMember(@path workspace: string, @path group: string, @path principal: string): AddGroupMemberOK | CommonErrors;

alias ListRoleBindingsOK = OkJson<RoleBindingListResponse>;

@tag("Access")
@summary("List role bindings")
@route("/workspaces/{workspace}/role-bindings")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listRoleBindings(@path workspace: string, @query subjectType?: string, @query subjectId?: string, @query role?: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListRoleBindingsOK | CommonErrors;

alias CreateRoleBindingCreated = CreatedJson<RoleBindingResponse>;

@tag("Access")
@summary("Create a role binding")
@route("/workspaces/{workspace}/role-bindings")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op createRoleBinding(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: RoleBindingRequest): CreateRoleBindingCreated | CommonErrors;

@tag("Access")
@summary("Get a workspace role binding")
@route("/workspaces/{workspace}/role-bindings/{binding}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getRoleBinding(@path workspace: string, @path binding: string): OkJson<RoleBindingResponse> | CommonErrors;

@tag("Access")
@summary("Delete a role binding")
@route("/workspaces/{workspace}/role-bindings/{binding}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op deleteRoleBinding(@path workspace: string, @path binding: string): NoContentResponse | CommonErrors;

alias UpdateRoleBindingOK = OkJson<RoleBindingResponse>;

@tag("Access")
@summary("Update a role binding")
@route("/workspaces/{workspace}/role-bindings/{binding}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updateRoleBinding(@path workspace: string, @path binding: string, @header("If-Match") ifMatch: string, @body body: RoleBindingRequest): UpdateRoleBindingOK | CommonErrors;

alias ListWorkspaceRolesOK = OkJson<RoleListResponse>;

@tag("Access")
@summary("List workspace roles")
@route("/workspaces/{workspace}/roles")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listWorkspaceRoles(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListWorkspaceRolesOK | CommonErrors;

alias ListEffectivePrivilegesOK = OkJson<EffectivePrivilegeListResponse>;

@tag("Access")
@summary("List effective privileges")
@route("/workspaces/{workspace}/effective-privileges")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op listEffectivePrivileges(@path workspace: string, @query objectType?: string, @query objectId?: string): ListEffectivePrivilegesOK | CommonErrors;

alias ListGrantsOK = OkJson<GrantListResponse>;

@tag("Access")
@summary("List grants")
@route("/workspaces/{workspace}/grants")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listGrants(@path workspace: string, @query objectType?: string, @query objectId?: string, @query includeInherited?: boolean, @query limit?: ListLimit, @query pageToken?: PageToken): ListGrantsOK | CommonErrors;

alias CreateGrantCreated = CreatedJson<GrantResponse>;

@tag("Access")
@summary("Create a grant")
@route("/workspaces/{workspace}/grants")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op createGrant(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: GrantRequest): CreateGrantCreated | CommonErrors;

@tag("Access")
@summary("Get an object privilege grant")
@route("/workspaces/{workspace}/grants/{grant}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getGrant(@path workspace: string, @path grant: string): OkJson<GrantResponse> | CommonErrors;

@tag("Access")
@summary("Update an object privilege grant")
@route("/workspaces/{workspace}/grants/{grant}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updateGrant(@path workspace: string, @path grant: string, @header("If-Match") ifMatch: string, @body body: GrantRequest): OkJson<GrantResponse> | CommonErrors;

@tag("Access")
@summary("Delete a grant")
@route("/workspaces/{workspace}/grants/{grant}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op deleteGrant(@path workspace: string, @path grant: string): NoContentResponse | CommonErrors;

alias TransferOwnershipOK = OkJson<SecurableObjectResponse>;

@tag("Access")
@summary("Transfer object ownership")
@route("/workspaces/{workspace}/ownership")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_ITEM" })
op transferOwnership(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: OwnershipTransferRequest): TransferOwnershipOK | CommonErrors;

alias ListDataPoliciesOK = OkJson<DataPolicyListResponse>;

@tag("Access")
@summary("List data policies")
@route("/workspaces/{workspace}/data-policies")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op listDataPolicies(@path workspace: string, @query objectType?: string, @query objectId?: string, @query includeInherited?: boolean, @query limit?: ListLimit, @query pageToken?: PageToken): ListDataPoliciesOK | CommonErrors;

alias CreateDataPolicyCreated = CreatedJson<DataPolicyResponse>;

@tag("Access")
@summary("Create a data policy")
@route("/workspaces/{workspace}/data-policies")
@post
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op createDataPolicy(@path workspace: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: DataPolicyRequest): CreateDataPolicyCreated | CommonErrors;

@tag("Access")
@summary("Get a data policy")
@route("/workspaces/{workspace}/data-policies/{policy}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op getDataPolicy(@path workspace: string, @path policy: string): OkJson<DataPolicyResponse> | CommonErrors;

@tag("Access")
@summary("Update a data policy")
@route("/workspaces/{workspace}/data-policies/{policy}")
@patch
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op updateDataPolicy(@path workspace: string, @path policy: string, @header("If-Match") ifMatch: string, @body body: DataPolicyRequest): OkJson<DataPolicyResponse> | CommonErrors;

@tag("Access")
@summary("Delete a data policy")
@route("/workspaces/{workspace}/data-policies/{policy}")
@delete
@apigen.authz(#{ mode: "privilege", privilege: "MANAGE_GRANTS" })
op deleteDataPolicy(@path workspace: string, @path policy: string): NoContentResponse | CommonErrors;

@tag("Access")
@summary("Check multiple authorization decisions")
@route("/workspaces/{workspace}/authorization-checks")
@post
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op checkAuthorizationBatch(@path workspace: string, @body body: AuthorizationBatchCheckRequest): OkJson<AuthorizationBatchCheckResponse> | CommonErrors;
