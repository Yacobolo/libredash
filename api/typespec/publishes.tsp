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
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["publishes", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "list_publishes",
  effect: "read",
  tags: #["publish"],
  input: #{ fields: #[
    #{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" },
    #{ source: "query", name: "limit", default: 25 },
  ] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/id" }, #{ source: "/environment" }, #{ source: "/status" }, #{ source: "/createdAt" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op listPublishes(@path workspace: string, @query environment?: string, @query limit?: int32, @query pageToken?: string): OkJson<PublishListResponse> | CommonErrors;

@tag("Publishes")
@summary("Create a publish")
@route("/workspaces/{workspace}/publishes")
@post
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op createPublish(@path workspace: string, @body body?: PublishCreateRequest): CreatedJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Get a publish")
@route("/workspaces/{workspace}/publishes/{publish}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.tool(#{
  name: "get_publish",
  effect: "read",
  tags: #["publish"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" }] },
  output: #{ mode: "project", select: #[#{ source: "/id" }, #{ source: "/environment" }, #{ source: "/status" }, #{ source: "/createdAt" }, #{ source: "/activatedAt" }, #{ source: "/error" }] },
})
op getPublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Activate a publish")
@route("/workspaces/{workspace}/publishes/{publish}/activate")
@post
@apigen.authz(#{ mode: "privilege", privilege: "ACTIVATE_PUBLISH" })
op activatePublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;

@tag("Publishes")
@summary("Upload a publish artifact")
@route("/workspaces/{workspace}/publishes/{publish}/artifact")
@put
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
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
@apigen.authz(#{ mode: "privilege", privilege: "DEPLOY" })
op validatePublish(@path workspace: string, @path publish: string, @query environment?: string): OkJson<PublishResponse> | CommonErrors;
