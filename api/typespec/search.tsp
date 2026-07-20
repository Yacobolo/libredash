using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

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
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["search"], args: #[#{ source: "query", name: "q", displayName: "query" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "search_workspace",
  effect: "read",
  tags: #["search", "discovery"],
  input: #{ fields: #[
    #{ source: "path", name: "workspace", mode: "context", contextKey: "workspace" },
    #{ source: "query", name: "limit", default: 10 },
  ] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[#{ source: "/type" }, #{ source: "/id" }, #{ source: "/name" }, #{ source: "/description" }] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op searchWorkspace(@path workspace: string, @query q?: string, @query types?: string, @query limit?: ListLimit, @query pageToken?: PageToken): SearchWorkspaceOK | CommonErrors;
