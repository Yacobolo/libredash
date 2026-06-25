using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model DeploymentCreateRequest {
  title?: string;
  description?: string;
}

model DeploymentArtifactResponse {
  deploymentId: string;
  sizeBytes: int64;
}


model DeploymentResponse {
  id: string;
  workspaceId: string;
  status: string;
  digest: string;
  createdAt: utcDateTime;
  activatedAt?: utcDateTime;
  error?: string;
}

model DeploymentListResponse {
  items: DeploymentResponse[];
  page: PageInfo;
}

@tag("Deployments")
@summary("List deployments")
@route("/workspaces/{workspace}/deployments")
@get
@apigen.authz(#{ mode: "permission", permission: "deployment:read" })
@apigen.cli(#{ command: #["deployments", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_deployments", risk: "read", tags: #["deployment"], defaultLimit: 25 })
op listDeployments(@path workspace: string, @query limit?: int32, @query pageToken?: string): OkJson<DeploymentListResponse> | CommonErrors;

@tag("Deployments")
@summary("Create a deployment")
@route("/workspaces/{workspace}/deployments")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
op createDeployment(@path workspace: string, @body body?: DeploymentCreateRequest): CreatedJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Get a deployment")
@route("/workspaces/{workspace}/deployments/{deployment}")
@get
@apigen.authz(#{ mode: "permission", permission: "deployment:read" })
@extension("x-agent", #{ enabled: true, name: "get_deployment", risk: "read", tags: #["deployment"] })
op getDeployment(@path workspace: string, @path deployment: string): OkJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Activate a deployment")
@route("/workspaces/{workspace}/deployments/{deployment}/activate")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:activate" })
op activateDeployment(@path workspace: string, @path deployment: string): OkJson<DeploymentResponse> | CommonErrors;

@tag("Deployments")
@summary("Upload a deployment artifact")
@route("/workspaces/{workspace}/deployments/{deployment}/artifact")
@put
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
@apigen.manual
op uploadDeploymentArtifact(
  @path workspace: string,
  @path deployment: string,
  @header contentType: "application/octet-stream",
  @body body: bytes,
): OkJson<DeploymentArtifactResponse> | CommonErrors;

@tag("Deployments")
@summary("Validate a deployment")
@route("/workspaces/{workspace}/deployments/{deployment}/validate")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
op validateDeployment(@path workspace: string, @path deployment: string): OkJson<DeploymentResponse> | CommonErrors;
