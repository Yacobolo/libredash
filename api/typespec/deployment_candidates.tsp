using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model DeploymentCandidateCreateRequest {
  title?: string;
  description?: string;
  environment: ProjectDeploymentEnvironmentId;
}

model DeploymentCandidateArtifactResponse {
  candidateId: DeploymentCandidateId;
  sizeBytes: int64;
}

model DeploymentCandidateResponse {
  id: DeploymentCandidateId;
  project: ProjectDeploymentProjectId;
  workspace: ProjectDeploymentWorkspaceId;
  environment: ProjectDeploymentEnvironmentId;
  status: string;
  digest: string;
  createdAt: utcDateTime;
  error?: string;
}

@tag("Deployments")
@summary("Create a workspace candidate for a project deployment")
@route("/projects/{project}/workspaces/{workspace}/deployment-candidates")
@post
@operationId("createDeploymentCandidate")
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op createDeploymentCandidate(
  @path project: ProjectDeploymentProjectId,
  @path workspace: ProjectDeploymentWorkspaceId,
  @body body: DeploymentCandidateCreateRequest,
): CreatedJson<DeploymentCandidateResponse> | CommonErrors;

@tag("Deployments")
@summary("Upload a project deployment candidate artifact")
@route("/projects/{project}/workspaces/{workspace}/deployment-candidates/{candidate}/artifact")
@put
@operationId("uploadDeploymentCandidateArtifact")
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
@apigen.manual
op uploadDeploymentCandidateArtifact(
  @path project: ProjectDeploymentProjectId,
  @path workspace: ProjectDeploymentWorkspaceId,
  @path candidate: DeploymentCandidateId,
  @header contentType: "application/octet-stream",
  @body body: bytes,
): OkJson<DeploymentCandidateArtifactResponse> | CommonErrors;

@tag("Deployments")
@summary("Validate a project deployment candidate")
@route("/projects/{project}/workspaces/{workspace}/deployment-candidates/{candidate}/validate")
@post
@operationId("validateDeploymentCandidate")
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op validateDeploymentCandidate(
  @path project: ProjectDeploymentProjectId,
  @path workspace: ProjectDeploymentWorkspaceId,
  @path candidate: DeploymentCandidateId,
): OkJson<DeploymentCandidateResponse> | CommonErrors;
