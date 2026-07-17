using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

enum ReleaseStatus {
  draft;
  validating;
  ready;
  failed;
}

model ReleaseWorkspaceManifest {
  workspace: string;
  artifactDigest: string;
  servingStateId?: string;
}

model ReleaseConnectionPin {
  connection: string;
  revisionId: string;
}

model ReleaseCreateRequest {
  projectDigest: string;
  workspaces: ReleaseWorkspaceManifest[];
  connections: ReleaseConnectionPin[];
}

model ReleaseResponse {
  id: string;
  projectId: string;
  projectDigest: string;
  status: ReleaseStatus;
  workspaces: ReleaseWorkspaceManifest[];
  connections: ReleaseConnectionPin[];
  createdBy: string;
  createdAt: utcDateTime;
  finalizedAt?: utcDateTime;
  error?: string;
}

model ReleaseListResponse {
  items: ReleaseResponse[];
  page: PageInfo;
}

model ReleaseArtifactResponse {
  releaseId: string;
  workspaceId: string;
  digest: string;
  sizeBytes: int64;
}

@tag("Releases")
@summary("List project releases")
@route("/projects/{project}/releases")
@get
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op listReleases(@path project: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<ReleaseListResponse> | CommonErrors;

@tag("Releases")
@summary("Create a draft project release")
@route("/projects/{project}/releases")
@post
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op createRelease(@path project: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey, @body body: ReleaseCreateRequest): CreatedJson<ReleaseResponse> | CommonErrors;

@tag("Releases")
@summary("Get a project release")
@route("/projects/{project}/releases/{release}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op getRelease(@path project: string, @path release: string): OkJson<ReleaseResponse> | CommonErrors;

@tag("Releases")
@summary("Upload an immutable workspace artifact")
@route("/projects/{project}/releases/{release}/workspaces/{workspace}/artifact")
@put
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
@apigen.manual
op uploadReleaseArtifact(@path project: string, @path release: string, @path workspace: string, @header contentType: "application/octet-stream", @header("Content-Digest") contentDigest: string, @body body: bytes): CreatedJson<ReleaseArtifactResponse> | CommonErrors;

@tag("Releases")
@summary("Finalize and validate a release")
@route("/projects/{project}/releases/{release}/finalize")
@post
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op finalizeRelease(@path project: string, @path release: string, @header("Idempotency-Key") idempotencyKey: IdempotencyKey): AcceptedJson<ReleaseResponse> | CommonErrors;

@tag("Releases")
@summary("List or stream release events")
@route("/projects/{project}/releases/{release}/events")
@get
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op listReleaseEvents(@path project: string, @path release: string, @header accept?: string, @header("Last-Event-ID") lastEventId?: string, @query limit?: ListLimit, @query pageToken?: PageToken): EventHistoryResponse<AsyncEventListResponse> | CommonErrors;
