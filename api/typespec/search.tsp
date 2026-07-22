using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

enum SearchResultType {
  workspace,
  connection,
  source,
  model_table,
  semantic_model,
  semantic_table,
  field,
  measure,
  dashboard,
  page,
  visual,
  filter,
  refresh_pipeline,
}

enum SearchContextTag {
  current_page,
  current_dashboard,
  current_workspace,
}

model SearchReference {
  workspaceId: string;
  type: SearchResultType;
  id: string;
}

model SearchWorkspaceSummary {
  id: string;
  name: string;
}

model SearchLocation {
  dashboardId?: string;
  dashboardName?: string;
  pageId?: string;
  pageName?: string;
  href: string;
}

model SearchResult {
  reference: SearchReference;
  name: string;
  description?: string;
  visualType?: string;
  workspace: SearchWorkspaceSummary;
  href: string;
  locations: SearchLocation[];
  context: SearchContextTag[];
}

model SearchResponse {
  items: SearchResult[];
  page: PageInfo;
}

alias SearchOK = OkJson<SearchResponse>;

@tag("Search")
@summary("Search accessible product objects")
@route("/search")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["search"], args: #[#{ source: "query", name: "q", displayName: "query" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "search",
  effect: "read",
  tags: #["search", "discovery"],
  input: #{ fields: #[#{ source: "query", name: "limit", default: 10 }] },
  output: #{
    mode: "project",
    select: #[#{ source: "/items", countAs: "count", select: #[
      #{ source: "/reference/workspaceId", target: "workspaceId" },
      #{ source: "/reference/type", target: "type" },
      #{ source: "/reference/id", target: "id" },
      #{ source: "/name" },
      #{ source: "/description" },
      #{ source: "/href" },
    ] }],
    cursor: #{ source: "/page/nextCursor" },
  },
})
op search(
  @query q?: string,
  @query workspace?: string[],
  @query type?: SearchResultType[],
  @query contextWorkspace?: string,
  @query contextDashboard?: string,
  @query contextPage?: string,
  @query limit?: ListLimit,
  @query pageToken?: PageToken,
): SearchOK | CommonErrors;
