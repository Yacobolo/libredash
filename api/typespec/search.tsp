using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model SearchResult {
  id: string;
  type: string;
  name: string;
  description: string;
  dashboardId?: string;
  pageId?: string;
  visualId?: string;
  tableId?: string;
  filterId?: string;
  modelId?: string;
  datasetId?: string;
  fieldId?: string;
  assetId?: string;
}

model SearchResponse {
  items: SearchResult[];
  page: PageInfo;
}

alias SearchWorkspaceOK = OkJson<SearchResponse>;

@tag("Search")
@summary("Search workspace")
@route("/workspaces/{workspace}/search")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["search"], args: #[#{ source: "query", name: "q", displayName: "query" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "search_workspace", risk: "read", tags: #["search", "discovery"], defaultLimit: 10, output: #{ itemsPath: "items", fields: #["type", "id", "name", "description"], cursorPath: "page.nextCursor", count: true } })
op searchWorkspace(@path workspace: string, @query q?: string, @query types?: string, @query limit?: int32, @query pageToken?: string): SearchWorkspaceOK | CommonErrors;
