using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model PublishCreateRequest {
  title?: string;
  description?: string;
  environment?: string;
}

model PublishArtifactResponse {
  publishId: string;
  sizeBytes: int64;
}

model PublishResponse {
  id: string;
  workspaceId: string;
  environment: string;
  status: string;
  digest: string;
  createdAt: utcDateTime;
  activatedAt?: utcDateTime;
  error?: string;
}

model PublishListResponse {
  items: PublishResponse[];
  page: PageInfo;
}

@tag("Publishes")
@summary("List publishes")
@route("/workspaces/{workspace}/publishes")
@get
@apigen.authz(#{ mode: "permission", permission: "publish:read" })
@apigen.cli(#{ command: #["publishes", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_publishes", risk: "read", tags: #["publish"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "environment", "status", "createdAt"], cursorPath: "page.nextCursor", count: true } })
op listPublishes(@path workspace: string, @query environment?: string, @query limit?: int32, @query pageToken?: string): OkJson<PublishListResponse> | CommonErrors;

@tag("Publishes")
@summary("Create a publish")
@route("/workspaces/{workspace}/publishes")
@post
@apigen.authz(#{ mode: "permission", permission: "publish:write" })
op createPublish(@path workspace: string, @body body?: PublishCreateRequest): CreatedJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Get a publish")
@route("/workspaces/{workspace}/publishes/{publish}")
@get
@apigen.authz(#{ mode: "permission", permission: "publish:read" })
@extension("x-agent", #{ enabled: true, name: "get_publish", risk: "read", tags: #["publish"], output: #{ rootFields: #["id", "environment", "status", "createdAt", "activatedAt", "error"] } })
op getPublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Activate a publish")
@route("/workspaces/{workspace}/publishes/{publish}/activate")
@post
@apigen.authz(#{ mode: "permission", permission: "publish:activate" })
op activatePublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Upload a publish artifact")
@route("/workspaces/{workspace}/publishes/{publish}/artifact")
@put
@apigen.authz(#{ mode: "permission", permission: "publish:write" })
@apigen.manual
op uploadPublishArtifact(
  @path workspace: string,
  @path publish: string,
  @header contentType: "application/octet-stream",
  @body body: bytes,
): OkJson<PublishArtifactResponse> | CommonErrors;

@tag("Publishes")
@summary("Validate a publish")
@route("/workspaces/{workspace}/publishes/{publish}/validate")
@post
@apigen.authz(#{ mode: "permission", permission: "publish:write" })
op validatePublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;
