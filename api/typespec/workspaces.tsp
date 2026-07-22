using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

model WorkspaceResponse {
  id: string;
  title: string;
  description: string;
  activeServingStateId?: string;
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
  servingStateId: string;
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
  servingStateId: string;
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
  servingStateId: string;
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
  servingStateId: string;
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
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
@apigen.cli(#{ command: #["workspaces", "list"], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "list_workspaces",
  effect: "read",
  tags: #["workspace"],
  input: #{ fields: #[#{ source: "query", name: "limit", default: 25 }] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/id" }, #{ source: "/title" }, #{ source: "/description" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op listWorkspaces(@query limit?: ListLimit, @query pageToken?: PageToken): ListWorkspacesOK | CommonErrors;

@tag("Workspaces")
@summary("Get a workspace")
@route("/workspaces/{workspace}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
op getWorkspace(@path workspace: string): OkJson<WorkspaceResponse> | CommonErrors;

alias ListWorkspaceAssetEdgesOK = OkJson<AssetEdgeListResponse>;

@tag("Workspaces")
@summary("List workspace asset edges")
@route("/workspaces/{workspace}/asset-edges")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
@apigen.cli(#{ command: #["assets", "edges"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "list_workspace_asset_edges",
  effect: "read",
  tags: #["workspace", "lineage"],
  input: #{ fields: #[
    #{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" },
    #{ source: "query", name: "limit", default: 25 },
  ] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/fromAssetId" }, #{ source: "/toAssetId" }, #{ source: "/type" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op listWorkspaceAssetEdges(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListWorkspaceAssetEdgesOK | CommonErrors;

alias ListWorkspaceAssetsOK = OkJson<AssetListResponse>;

@tag("Workspaces")
@summary("List workspace assets")
@route("/workspaces/{workspace}/assets")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
@apigen.cli(#{ command: #["assets", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "list_assets",
  effect: "read",
  tags: #["workspace", "asset", "lineage"],
  input: #{ fields: #[
    #{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" },
    #{ source: "query", name: "limit", default: 25 },
  ] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/id" }, #{ source: "/type" }, #{ source: "/title" }, #{ source: "/description" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op listWorkspaceAssets(@path workspace: string, @query type?: string, @query q?: string, @query limit?: ListLimit, @query pageToken?: PageToken): ListWorkspaceAssetsOK | CommonErrors;

@tag("Workspaces")
@summary("Get the active asset graph")
@route("/workspaces/{workspace}/active-asset-graph")
@get
@apigen.authz(#{ mode: "privilege", privilege: "USE_WORKSPACE" })
op getWorkspaceActiveAssetGraph(@path workspace: string): OkJson<WorkspaceAssetGraphResponse> | CommonErrors;

@tag("Workspaces")
@summary("Describe a workspace asset")
@route("/workspaces/{workspace}/assets/{assetId}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "workspace-asset")
@apigen.cli(#{ command: #["assets", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "assetId", displayName: "asset-id" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "describe_asset",
  effect: "read",
  tags: #["workspace", "asset"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" }] },
  output: #{ mode: "project", select: #[#{ source: "/id" }, #{ source: "/type" }, #{ source: "/title" }, #{ source: "/description" }, #{ source: "/payloadSchema" }, #{ source: "/href" }] },
})
op getWorkspaceAsset(@path workspace: string, @path assetId: string): OkJson<AssetResponse> | CommonErrors;

@tag("Workspaces")
@summary("Get workspace asset lineage")
@route("/workspaces/{workspace}/assets/{assetId}/lineage")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "workspace-asset")
@apigen.cli(#{ command: #["assets", "lineage"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "assetId", displayName: "asset-id" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "asset_lineage",
  effect: "read",
  tags: #["workspace", "asset", "lineage"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" }] },
  output: #{ mode: "project", select: #[#{ source: "/assetId" }, #{ source: "/upstream" }, #{ source: "/downstream" }] },
})
op getWorkspaceAssetLineage(@path workspace: string, @path assetId: string): OkJson<AssetLineageResponse> | CommonErrors;
