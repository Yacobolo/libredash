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
  payloadSchema: string;
  payload: Record<string>;
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

alias ListWorkspacesOK = OkJson<WorkspaceListResponse>;

@tag("Workspaces")
@summary("List workspaces")
@route("/workspaces")
@get
@apigen.authz(#{ mode: "permission", permission: "workspace:read" })
@apigen.cli(#{ command: #["workspaces", "list"], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_workspaces", risk: "read", tags: #["workspace"], defaultLimit: 25 })
op listWorkspaces(@query limit?: int32, @query pageToken?: string): ListWorkspacesOK | CommonErrors;

alias ListWorkspaceAssetEdgesOK = OkJson<AssetEdgeListResponse>;

@tag("Workspaces")
@summary("List workspace asset edges")
@route("/workspaces/{workspace}/asset-edges")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@extension("x-agent", #{ enabled: true, name: "list_workspace_asset_edges", risk: "read", tags: #["workspace", "lineage"], defaultLimit: 25 })
op listWorkspaceAssetEdges(@path workspace: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetEdgesOK | CommonErrors;

alias ListWorkspaceAssetsOK = OkJson<AssetListResponse>;

@tag("Workspaces")
@summary("List workspace assets")
@route("/workspaces/{workspace}/assets")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@extension("x-agent", #{ enabled: true, name: "list_workspace_assets", risk: "read", tags: #["workspace", "asset", "lineage"], defaultLimit: 25 })
op listWorkspaceAssets(@path workspace: string, @query type?: string, @query q?: string, @query limit?: int32, @query pageToken?: string): ListWorkspaceAssetsOK | CommonErrors;
