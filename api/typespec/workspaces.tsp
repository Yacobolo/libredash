using Http;
using TypeSpec.OpenAPI;

namespace LibreDashAPI;

model WorkspaceResponse {
  id: string;
  title: string;
  description: string;
  activeDeploymentId?: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model WorkspaceListResponse {
  items: WorkspaceResponse[];
  page: PageInfo;
}

model AssetResponse {
  id: string;
  workspaceId: string;
  deploymentId: string;
  type: string;
  key: string;
  parentId?: string;
  title: string;
  description: string;
  meta?: Record<string>;
  href?: string;
}

model AssetListResponse {
  items: AssetResponse[];
  page: PageInfo;
}

model AssetEdgeResponse {
  id: string;
  workspaceId: string;
  deploymentId: string;
  fromAssetId: string;
  toAssetId: string;
  type: string;
}

model AssetEdgeListResponse {
  items: AssetEdgeResponse[];
  page: PageInfo;
}

model ListWorkspacesOK {
  @statusCode statusCode: 200;
  @body body: WorkspaceListResponse;
}

@tag("Workspaces")
@summary("List workspaces")
@route("/api/v1/workspaces")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
@apigen.cli(#{ command: #["workspaces", "list"], output: #{ mode: "raw" } })
op listWorkspaces(@query limit?: int32, @query pageToken?: string): ListWorkspacesOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListWorkspaceAssetEdgesOK {
  @statusCode statusCode: 200;
  @body body: AssetEdgeListResponse;
}

@tag("Workspaces")
@summary("List workspace asset edges")
@route("/api/v1/workspaces/{workspace}/asset-edges")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
op listWorkspaceAssetEdges(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetEdgesOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;

model ListWorkspaceAssetsOK {
  @statusCode statusCode: 200;
  @body body: AssetListResponse;
}

@tag("Workspaces")
@summary("List workspace assets")
@route("/api/v1/workspaces/{workspace}/assets")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
op listWorkspaceAssets(@path workspace: string, @query type?: string, @query q?: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetsOK | BadRequest | Unauthorized | Forbidden | NotFound | Conflict | RateLimited | InternalServerError;
