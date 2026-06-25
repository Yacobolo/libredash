using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model DeploymentCreateRequest {
  title?: string;
  description?: string;
}

model DeploymentArtifactResponse {
  deploymentId: string;
  sizeBytes: int64;
}

model DeploymentArtifactUploadRequest {
  value: bytes;
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

model ListDeploymentsOK {
  @statusCode statusCode: 200;
  @body body: DeploymentListResponse;
}

@tag("Deployments")
@summary("List deployments")
@route("/api/v1/workspaces/{workspace}/deployments")
@get
@apigen.authz(#{ mode: "permission", permission: "deployment:read" })
@apigen.cli(#{ command: #["deployments", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listDeployments(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListDeploymentsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model CreateDeploymentCreated {
  @statusCode statusCode: 201;
  @body body: DeploymentResponse;
}

@tag("Deployments")
@summary("Create a deployment")
@route("/api/v1/workspaces/{workspace}/deployments")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
op createDeployment(@path workspace: string, @body body?: DeploymentCreateRequest): CreateDeploymentCreated | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model GetDeploymentOK {
  @statusCode statusCode: 200;
  @body body: DeploymentResponse;
}

@tag("Deployments")
@summary("Get a deployment")
@route("/api/v1/workspaces/{workspace}/deployments/{deployment}")
@get
@apigen.authz(#{ mode: "permission", permission: "deployment:read" })
op getDeployment(@path workspace: string, @path deployment: string): GetDeploymentOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ActivateDeploymentOK {
  @statusCode statusCode: 200;
  @body body: DeploymentResponse;
}

@tag("Deployments")
@summary("Activate a deployment")
@route("/api/v1/workspaces/{workspace}/deployments/{deployment}/activate")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:activate" })
op activateDeployment(@path workspace: string, @path deployment: string): ActivateDeploymentOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model UploadDeploymentArtifactOK {
  @statusCode statusCode: 200;
  @body body: DeploymentArtifactResponse;
}

@tag("Deployments")
@summary("Upload a deployment artifact")
@route("/api/v1/workspaces/{workspace}/deployments/{deployment}/artifact")
@put
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
@extension("x-libredash-dispatch", "raw-body")
op uploadDeploymentArtifact(@path workspace: string, @path deployment: string, @body body: DeploymentArtifactUploadRequest): UploadDeploymentArtifactOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ValidateDeploymentOK {
  @statusCode statusCode: 200;
  @body body: DeploymentResponse;
}

@tag("Deployments")
@summary("Validate a deployment")
@route("/api/v1/workspaces/{workspace}/deployments/{deployment}/validate")
@post
@apigen.authz(#{ mode: "permission", permission: "deployment:write" })
op validateDeployment(@path workspace: string, @path deployment: string): ValidateDeploymentOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
