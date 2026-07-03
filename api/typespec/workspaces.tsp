using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
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
  snapshotId: string;
  workspaceId: string;
  deploymentId: string;
  type: string;
  key: string;
  parentId?: string;
  title: string;
  description: string;
  sourceFile?: string;
  payloadSchema: string;
  payload: JSONObject;
  href?: string;
}

model AssetSummaryResponse {
  id: string;
  snapshotId: string;
  workspaceId: string;
  deploymentId: string;
  type: string;
  key: string;
  parentId?: string;
  title: string;
  description: string;
  sourceFile?: string;
  payloadSchema: string;
  contentHash: string;
  href?: string;
}

model AssetGraphAssetResponse {
  id: string;
  snapshotId: string;
  workspaceId: string;
  deploymentId: string;
  type: string;
  key: string;
  parentId?: string;
  title: string;
  description: string;
  sourceFile?: string;
  payloadSchema: string;
  payload: JSONObject;
  contentHash: string;
}

model WorkspaceAssetGraphResponse {
  assets: AssetGraphAssetResponse[];
  edges: AssetEdgeResponse[];
}

model AssetListResponse {
  items: AssetSummaryResponse[];
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

model AssetLineageResponse {
  assetId: string;
  upstream: string[];
  downstream: string[];
}

alias ListWorkspacesOK = OkJson<WorkspaceListResponse>;

@tag("Workspaces")
@summary("List workspaces")
@route("/workspaces")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
@apigen.cli(#{ command: #["workspaces", "list"], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_workspaces", risk: "read", tags: #["workspace"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "title", "description"], cursorPath: "page.nextCursor", count: true } })
op listWorkspaces(@query environment?: string, @query limit?: int32, @query pageToken?: string): ListWorkspacesOK | CommonErrors;

alias ListWorkspaceAssetEdgesOK = OkJson<AssetEdgeListResponse>;

@tag("Workspaces")
@summary("List workspace asset edges")
@route("/workspaces/{workspace}/asset-edges")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["assets", "edges"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_workspace_asset_edges", risk: "read", tags: #["workspace", "lineage"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["fromAssetId", "toAssetId", "type"], cursorPath: "page.nextCursor", count: true } })
op listWorkspaceAssetEdges(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetEdgesOK | CommonErrors;

alias ListWorkspaceAssetsOK = OkJson<AssetListResponse>;

@tag("Workspaces")
@summary("List workspace assets")
@route("/workspaces/{workspace}/assets")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["assets", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_assets", risk: "read", tags: #["workspace", "asset", "lineage"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "type", "title", "description"], cursorPath: "page.nextCursor", count: true } })
op listWorkspaceAssets(@path workspace: string, @query type?: string, @query q?: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetsOK | CommonErrors;

@tag("Workspaces")
@summary("Get the active deployment asset graph")
@route("/workspaces/{workspace}/active-deployment/graph")
@get
@apigen.authz(#{ mode: "permission", permission: "deployment:read" })
op getWorkspaceActiveDeploymentGraph(@path workspace: string, @query environment?: string): OkJson<WorkspaceAssetGraphResponse> | CommonErrors;

@tag("Workspaces")
@summary("Describe a workspace asset")
@route("/workspaces/{workspace}/assets/{assetId}")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["assets", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "assetId", displayName: "asset-id" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "describe_asset", risk: "read", tags: #["workspace", "asset"], output: #{ rootFields: #["id", "type", "title", "description", "payloadSchema", "href"] } })
op getWorkspaceAsset(@path workspace: string, @path assetId: string): OkJson<AssetResponse> | CommonErrors;

@tag("Workspaces")
@summary("Get workspace asset lineage")
@route("/workspaces/{workspace}/assets/{assetId}/lineage")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["assets", "lineage"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "assetId", displayName: "asset-id" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "asset_lineage", risk: "read", tags: #["workspace", "asset", "lineage"], output: #{ rootFields: #["assetId", "upstream", "downstream"] } })
op getWorkspaceAssetLineage(@path workspace: string, @path assetId: string): OkJson<AssetLineageResponse> | CommonErrors;
