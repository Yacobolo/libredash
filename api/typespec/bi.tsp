using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LibreDashAPI;

model DashboardSummary {
  id: string;
  title: string;
  description: string;
  semanticModel: string;
  tags: string[];
  pageCount: int32;
}

model DashboardListResponse {
  items: DashboardSummary[];
  page: PageInfo;
}

model SemanticModelSummary {
  id: string;
  title: string;
  description: string;
}

model SemanticModelListResponse {
  items: SemanticModelSummary[];
  page: PageInfo;
}

model ModelRef {
  id: string;
  title: string;
}

model DashboardManifestCounts {
  pages: int32;
  visuals: int32;
  tables: int32;
  filters: int32;
}

model DashboardManifestComponent {
  id: string;
  kind: string;
  ref: string;
  title?: string;
}

model DashboardComponentPlacement {
  col?: int32;
  row?: int32;
  colSpan?: int32;
  rowSpan?: int32;
}

model DashboardComponentResponse {
  id: string;
  kind: string;
  ref?: string;
  title?: string;
  description?: string;
  placement?: DashboardComponentPlacement;
  x?: float64;
  y?: float64;
  width?: float64;
  height?: float64;
}

model DashboardComponentListResponse {
  items: DashboardComponentResponse[];
  page: PageInfo;
}

model DashboardVisualDescribeResponse {
  id: string;
  componentId?: string;
  kind?: string;
  shape?: string;
  renderer?: string;
  type?: string;
  title?: string;
  description?: string;
  query?: Record<string>;
  options?: Record<string>;
  rendererOptions?: Record<string>;
  interaction?: Record<string>;
  placement?: DashboardComponentPlacement;
  x?: float64;
  y?: float64;
  width?: float64;
  height?: float64;
}

model DashboardManifestPage {
  id: string;
  title: string;
  description?: string;
  components: DashboardManifestComponent[];
}

model DashboardManifestResponse {
  id: string;
  title: string;
  description?: string;
  `semantic_model`?: string;
  `model`?: ModelRef;
  counts: DashboardManifestCounts;
  pages: DashboardManifestPage[];
  `detail_tools`: Record<string>;
}

model SemanticModelCounts {
  sources: int32;
  `model_tables`: int32;
  fields: int32;
  measures: int32;
  relationships: int32;
}

model SemanticModelTableSummary {
  id: string;
  kind: string;
  source: string;
  description: string;
  fields: int32;
}

model SemanticDatasetSummary {
  id: string;
  kind: string;
  source: string;
  description: string;
  fieldCount: int32;
  measureCount: int32;
}

model SemanticDatasetListResponse {
  items: SemanticDatasetSummary[];
  page: PageInfo;
}

model SemanticDatasetResponse {
  id: string;
  kind: string;
  source: string;
  sources: string[];
  description: string;
  primaryKey: string;
  grain: string;
  fieldCount: int32;
  measureCount: int32;
}

enum SemanticFieldKind {
  dimension;
  measure;
}

model SemanticFieldResponse {
  id: string;
  kind: SemanticFieldKind;
  table: string;
  name: string;
  label: string;
  description?: string;
  unit?: string;
  format?: string;
  grain?: string;
  time?: string;
  grains?: string[];
}

model SemanticFieldListResponse {
  items: SemanticFieldResponse[];
  page: PageInfo;
}

model SemanticFieldRef {
  field: string;
  `alias`?: string;
}

model SemanticTimeRef {
  field: string;
  grain?: string;
  `alias`?: string;
}

model SemanticSort {
  field: string;
  direction?: string;
}

model SemanticQueryRequest {
  dimensions?: SemanticFieldRef[];
  measures?: SemanticFieldRef[];
  time?: SemanticTimeRef;
  filters?: Record<string>[];
  sort?: SemanticSort[];
  limit?: int32;
  pageToken?: string;
}

model SemanticPreviewRequest {
  dimensions?: SemanticFieldRef[];
  measures?: SemanticFieldRef[];
  filters?: Record<string>[];
  sort?: SemanticSort[];
  limit?: int32;
  pageToken?: string;
}

model SemanticQueryRow {}

model SemanticQueryResponse {
  columns: string[];
  items: SemanticQueryRow[];
  page: PageInfo;
}

model SemanticExplainResponse {
  mode: string;
  sql: string;
  args: Record<string>[];
  columns: string[];
  warnings: string[];
}

model ModelDashboardUsage {
  id: string;
  title: string;
  `semantic_model`: string;
  pages: int32;
}

model SemanticModelDescriptionResponse {
  id: string;
  title: string;
  description: string;
  dashboards: ModelDashboardUsage[];
  counts?: SemanticModelCounts;
  tables?: SemanticModelTableSummary[];
}

model DashboardFilters {
  controls?: Record<string>;
  selections?: Record<string>[];
}

model DashboardPageQueryRequest {
  filters?: DashboardFilters;
}

model DashboardTableQueryRequest {
  pageId?: string;
  count?: int32;
  filters?: DashboardFilters;
}

model DashboardTableDataRequest {
  count?: int32;
  filters?: DashboardFilters;
}

model DashboardPageQueryResponse {
  filters: Record<string>;
  filterOptions?: Record<string>;
  status?: Record<string>;
  visuals: Record<string>;
}

model DashboardVisualDatum {}

model DashboardVisualDataResponse {
  version?: int32;
  id?: string;
  kind?: string;
  shape?: string;
  renderer?: string;
  type?: string;
  title?: string;
  unit?: string;
  format?: string;
  interaction?: Record<string>;
  dimensions?: string[];
  measure?: string;
  measures?: string[];
  series?: string[];
  options?: Record<string>;
  rendererOptions?: Record<string>;
  selection?: Record<string>[];
  data: DashboardVisualDatum[];
}

model DashboardTableQueryResponse {
  title: string;
  availableRows: int32;
  blocks: Record<string>;
}

model DashboardFilterOptionResponse {
  value: string;
  label: string;
}

model DashboardFilterOptionListResponse {
  items: DashboardFilterOptionResponse[];
  page: PageInfo;
}

@tag("BI")
@summary("List dashboards")
@route("/workspaces/{workspace}/dashboards")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_dashboards", risk: "read", tags: #["dashboard", "catalog"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "title", "description", "pageCount"], cursorPath: "page.nextCursor", count: true } })
op listDashboards(@path workspace: string, @query limit?: int32, @query pageToken?: string): OkJson<DashboardListResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard")
@route("/workspaces/{workspace}/dashboards/{dashboard}")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "describe_dashboard", risk: "read", tags: #["dashboard", "catalog"], output: #{ rootFields: #["id", "title", "description", "semantic_model", "model", "counts", "pages", "detail_tools"] } })
op getDashboard(@path workspace: string, @path dashboard: string): OkJson<DashboardManifestResponse> | CommonErrors;

@tag("BI")
@summary("List dashboard page components")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/components")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "components"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_dashboard_components", risk: "read", tags: #["dashboard", "visual"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "kind", "ref", "title"], cursorPath: "page.nextCursor", count: true } })
op listDashboardComponents(@path workspace: string, @path dashboard: string, @path page: string, @query limit?: int32, @query pageToken?: string): OkJson<DashboardComponentListResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard visual")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "visual"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "visual", displayName: "visual" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "describe_dashboard_visual", risk: "read", tags: #["dashboard", "visual"], output: #{ rootFields: #["id", "title", "description", "kind", "shape", "type", "query"] } })
op getDashboardVisual(@path workspace: string, @path dashboard: string, @path page: string, @path visual: string): OkJson<DashboardVisualDescribeResponse> | CommonErrors;

@tag("BI")
@summary("List semantic models")
@route("/workspaces/{workspace}/semantic-models")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_semantic_models", risk: "read", tags: #["semantic-model", "catalog"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "title", "description"], cursorPath: "page.nextCursor", count: true } })
op listSemanticModels(@path workspace: string, @query limit?: int32, @query pageToken?: string): OkJson<SemanticModelListResponse> | CommonErrors;

@tag("BI")
@summary("Describe semantic model")
@route("/workspaces/{workspace}/semantic-models/{model}")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "describe_model", risk: "read", tags: #["semantic-model", "catalog"], output: #{ rootFields: #["id", "title", "description", "dashboards", "counts", "tables"] } })
op getSemanticModel(@path workspace: string, @path("model") modelName: string): OkJson<SemanticModelDescriptionResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model datasets")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "datasets"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_semantic_datasets", risk: "read", tags: #["semantic-model", "dataset"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["id", "kind", "description"], cursorPath: "page.nextCursor", count: true } })
op listSemanticDatasets(@path workspace: string, @path("model") modelName: string, @query limit?: int32, @query pageToken?: string): OkJson<SemanticDatasetListResponse> | CommonErrors;

@tag("BI")
@summary("Describe semantic model dataset")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "dataset"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "describe_semantic_dataset", risk: "read", tags: #["semantic-model", "dataset"], output: #{ rootFields: #["id", "kind", "source", "sources", "description", "primaryKey", "grain", "fieldCount", "measureCount"] } })
op getSemanticDataset(@path workspace: string, @path("model") modelName: string, @path dataset: string): OkJson<SemanticDatasetResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model dataset fields")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields")
@get
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "fields"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_semantic_fields", risk: "read", tags: #["semantic-model", "dataset", "field"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["kind", "id", "label", "description"], cursorPath: "page.nextCursor", count: true } })
op listSemanticFields(@path workspace: string, @path("model") modelName: string, @path dataset: string, @query limit?: int32, @query pageToken?: string): OkJson<SemanticFieldListResponse> | CommonErrors;

@tag("BI")
@summary("Query semantic model dataset")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/query")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "query"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "query_semantic_dataset", risk: "read", tags: #["semantic-model", "dataset", "query"], defaultLimit: 25, output: #{ rootFields: #["columns"], collections: #[#{ path: "items", as: "items", count: true }], cursorPath: "page.nextCursor" } })
op querySemanticDataset(@path workspace: string, @path("model") modelName: string, @path dataset: string, @body body?: SemanticQueryRequest): OkJson<SemanticQueryResponse> | CommonErrors;

@tag("BI")
@summary("Preview semantic model dataset rows")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "preview"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "preview_semantic_dataset", risk: "read", tags: #["semantic-model", "dataset", "query"], defaultLimit: 25, output: #{ rootFields: #["columns"], collections: #[#{ path: "items", as: "items", count: true }], cursorPath: "page.nextCursor" } })
op previewSemanticDataset(@path workspace: string, @path("model") modelName: string, @path dataset: string, @body body?: SemanticPreviewRequest): OkJson<SemanticQueryResponse> | CommonErrors;

@tag("BI")
@summary("Explain semantic model dataset query")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/query/explain")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "explain-query"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "explain_semantic_query", risk: "read", tags: #["semantic-model", "dataset", "query"], output: #{ rootFields: #["mode", "sql", "args", "columns", "warnings"] } })
op explainSemanticQuery(@path workspace: string, @path("model") modelName: string, @path dataset: string, @body body?: SemanticQueryRequest): OkJson<SemanticExplainResponse> | CommonErrors;

@tag("BI")
@summary("Explain semantic model dataset row preview")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview/explain")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["semantic-models", "explain-preview"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "explain_semantic_preview", risk: "read", tags: #["semantic-model", "dataset", "query"], output: #{ rootFields: #["mode", "sql", "args", "columns", "warnings"] } })
op explainSemanticPreview(@path workspace: string, @path("model") modelName: string, @path dataset: string, @body body?: SemanticPreviewRequest): OkJson<SemanticExplainResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard page")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/query")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "query-page"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "query_dashboard_page", risk: "read", tags: #["dashboard", "query"], output: #{ maps: #[#{ path: "visuals", as: "visuals", fields: #["id", "title", "kind", "shape", "type", "unit", "format", "dimensions", "measure", "measures", "series"], collection: #{ path: "data", as: "data", count: true } }] } })
op queryDashboardPage(@path workspace: string, @path dashboard: string, @path page: string, @body body?: DashboardPageQueryRequest): OkJson<DashboardPageQueryResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard visual data")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}/data")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "visual-data"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "visual", displayName: "visual" }], output: #{ mode: "collection" }, pagination: #{ itemsField: "data" } })
@extension("x-agent", #{ enabled: true, name: "query_dashboard_visual_data", risk: "read", tags: #["dashboard", "visual", "query"], output: #{ rootFields: #["title", "type", "shape", "unit", "format", "dimensions", "measure", "measures", "series"], collections: #[#{ path: "data", as: "data", count: true }] } })
op queryDashboardVisualData(@path workspace: string, @path dashboard: string, @path page: string, @path visual: string, @body body?: DashboardPageQueryRequest): OkJson<DashboardVisualDataResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard table")
@route("/workspaces/{workspace}/dashboards/{dashboard}/tables/{table}/query")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "query-table"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "table", displayName: "table" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "query_table", risk: "read", tags: #["dashboard", "query", "table"], output: #{ rootFields: #["title", "availableRows"], collections: #[#{ path: "columns", as: "columns", fields: #["key", "label", "role", "format"] }, #{ path: "blocks.a.rows", as: "rows", count: true }] } })
op queryDashboardTable(@path workspace: string, @path dashboard: string, @path table: string, @body body?: DashboardTableQueryRequest): OkJson<DashboardTableQueryResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard table data")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/tables/{table}/data")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "table-data"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "table", displayName: "table" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "query_dashboard_table_data", risk: "read", tags: #["dashboard", "query", "table"], output: #{ rootFields: #["title", "availableRows"], collections: #[#{ path: "columns", as: "columns", fields: #["key", "label", "role", "format"] }, #{ path: "blocks.a.rows", as: "rows", count: true }] } })
op queryDashboardTableData(@path workspace: string, @path dashboard: string, @path page: string, @path table: string, @body body?: DashboardTableDataRequest): OkJson<DashboardTableQueryResponse> | CommonErrors;

@tag("BI")
@summary("List dashboard filter options")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/options")
@post
@apigen.authz(#{ mode: "permission", permission: "asset:read" })
@apigen.cli(#{ command: #["dashboards", "filter-options"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "filter", displayName: "filter" }], output: #{ mode: "raw" } })
@extension("x-agent", #{ enabled: true, name: "list_dashboard_filter_options", risk: "read", tags: #["dashboard", "filter"], defaultLimit: 25, output: #{ itemsPath: "items", fields: #["value", "label"], cursorPath: "page.nextCursor", count: true } })
op listDashboardFilterOptions(@path workspace: string, @path dashboard: string, @path page: string, @path filter: string, @query limit?: int32, @query pageToken?: string, @body body?: DashboardPageQueryRequest): OkJson<DashboardFilterOptionListResponse> | CommonErrors;
