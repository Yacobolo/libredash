using Http;
using TypeSpec.OpenAPI;

@route("/api/v1")
namespace LeapViewAPI;

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
  filters: int32;
}

model DashboardManifestComponent {
  id: string;
  kind: "visual" | "filter" | "header";
  ref: string;
  title?: string;
}

model DashboardComponentPlacement {
  col?: int32;
  row?: int32;
  colSpan?: int32;
  rowSpan?: int32;
}

@discriminator("kind")
model DashboardComponentResponse {
  id: string;
  kind: "visual" | "filter" | "header";
  ref?: string;
  title?: string;
  description?: string;
  placement?: DashboardComponentPlacement;
  x?: float64;
  y?: float64;
  width?: float64;
  height?: float64;
}

model DashboardVisualComponentResponse extends DashboardComponentResponse {
  kind: "visual";
  visualId: string;
}

model DashboardFilterComponentResponse extends DashboardComponentResponse {
  kind: "filter";
  filterId: string;
}

model DashboardHeaderComponentResponse extends DashboardComponentResponse {
  kind: "header";
}

model DashboardPageResponse {
  id: string;
  title: string;
  description?: string;
  components: DashboardComponentResponse[];
}

model DashboardVisualDescribeResponse {
  id: string;
  componentId?: string;
  rendererID: string;
  specRevision: string;
  spec: LeapViewVisualization.VisualizationSpec;
  placement?: DashboardComponentPlacement;
  x?: float64;
  y?: float64;
  width?: float64;
  height?: float64;
}

model DashboardFilterDescribeResponse {
  id: string;
  componentId?: string;
  title?: string;
  description?: string;
  field?: string;
  multiSelect?: boolean;
  placement?: DashboardComponentPlacement;
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
  facts: int32;
  `conformed_dimensions`: int32;
  `atomic_measures`: int32;
  metrics: int32;
  relationships: int32;
}

model SemanticModelTableSummary {
  id: string;
  roles: string[];
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
  metric;
}

model SemanticFieldResponse {
  id: string;
  kind: SemanticFieldKind;
  table: string;
  name: string;
  label: string;
  description?: string;
  type?: string;
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

model SemanticRelationshipResponse {
  id: string;
  fromDataset: string;
  fromField: string;
  toDataset: string;
  toField: string;
  cardinality: string;
  active: boolean;
}

model SemanticRelationshipListResponse {
  items: SemanticRelationshipResponse[];
  page: PageInfo;
}

model SemanticSourceResponse {
  id: string;
  kind: string;
  connection?: string;
  table?: string;
  path?: string;
  description?: string;
}

model SemanticSourceListResponse {
  items: SemanticSourceResponse[];
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

model SemanticFilterGroup {
  filters: SemanticFilter[];
}

model SemanticFilter {
  field?: string;
  fact?: string;
  operator?: string;
  values?: string[];
  groups?: SemanticFilterGroup[];
}

model SemanticQueryRequest {
  @maxItems(50)
  dimensions?: SemanticFieldRef[];
  @maxItems(50)
  measures?: SemanticFieldRef[];
  time?: SemanticTimeRef;
  @maxItems(100)
  filters?: SemanticFilter[];
  @maxItems(50)
  sort?: SemanticSort[];
  limit?: QueryLimit;
  pageToken?: PageToken;
}

model SemanticPreviewRequest {
  @maxItems(50)
  dimensions?: SemanticFieldRef[];
  @maxItems(50)
  measures?: SemanticFieldRef[];
  @maxItems(100)
  filters?: SemanticFilter[];
  @maxItems(50)
  sort?: SemanticSort[];
  limit?: QueryLimit;
  pageToken?: PageToken;
}

enum QueryColumnType {
  boolean;
  int64;
  decimal;
  float64;
  string;
  date;
  timestamp;
  json;
}

model QueryColumn {
  name: string;
  type: QueryColumnType;
  nullable: boolean;
}

// Cells are encoded as strings so int64 and decimal values never lose precision.
// The typed column descriptor determines how clients interpret each position;
// an empty string represents SQL NULL and is disambiguated by nullable metadata.
alias SemanticQueryRow = string[];

model SemanticQueryResponse {
  queryId: string;
  servingSnapshot: string;
  columns: QueryColumn[];
  rows: SemanticQueryRow[];
  page: PageInfo;
}

model SemanticExplainResponse {
  mode: string;
  facts: string[];
  stitchDimensions: string[];
  physicalDependencies: string[];
  relationshipPaths: string[];
  sql: string;
  args: Record<unknown>[];
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
  controls?: Record<unknown>;
  selections?: Record<unknown>[];
  spatialSelections?: DashboardSpatialInteractionSelection[];
}

model DashboardSpatialInteractionSelection {
  visualID: string;
  interactionID: string;
  gesture: LeapViewVisualization.VisualizationSpatialSelectionGesture;
  geometry: DashboardSpatialSelectionGeometry;
  order: int32;
}

model DashboardSpatialSelectionGeometry {
  kind: "box" | "lasso" | "radius";
  bounds?: LeapViewVisualization.VisualizationSpatialBounds;
  points?: LeapViewVisualization.VisualizationSpatialCoordinate[];
  center?: LeapViewVisualization.VisualizationSpatialCoordinate;
  radiusMeters?: float64;
}

model DashboardPageQueryRequest {
  filters?: DashboardFilters;
}

model DashboardVisualQueryRequest {
  limit?: QueryLimit;
  pageToken?: PageToken;
  filters?: DashboardFilters;
}

model DashboardPageQueryResponse {
  filters: Record<unknown>;
  filterOptions?: Record<unknown>;
  status?: Record<unknown>;
  visuals: Record<LeapViewVisualization.VisualizationEnvelope>;
}

model DashboardTableColumn {
  key: string;
  label: string;
  role?: string;
  format?: string;
}

model DashboardTableQueryResponse {
  type: "table" | "matrix" | "pivot";
  queryId: string;
  servingSnapshot: string;
  title: string;
  columns: QueryColumn[];
  rows: string[][];
  availableRows: int32;
  page: PageInfo;
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
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["dashboards", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listDashboards(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<DashboardListResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard")
@route("/workspaces/{workspace}/dashboards/{dashboard}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "dashboard")
@apigen.cli(#{ command: #["dashboards", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }], output: #{ mode: "raw" } })
op getDashboard(@path workspace: string, @path dashboard: string): OkJson<DashboardManifestResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard page")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "dashboard")
@apigen.cli(#{ command: #["dashboards", "page"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }], output: #{ mode: "raw" } })
op getDashboardPage(@path workspace: string, @path dashboard: string, @path page: string): OkJson<DashboardPageResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard visual")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "dashboard")
@apigen.cli(#{ command: #["dashboards", "visual"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "visual", displayName: "visual" }], output: #{ mode: "raw" } })
op getDashboardVisual(@path workspace: string, @path dashboard: string, @path page: string, @path visual: string): OkJson<DashboardVisualDescribeResponse> | CommonErrors;

@tag("BI")
@summary("Describe dashboard filter")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "dashboard")
op getDashboardFilter(@path workspace: string, @path dashboard: string, @path page: string, @path filter: string): OkJson<DashboardFilterDescribeResponse> | CommonErrors;

@tag("BI")
@summary("List semantic models")
@route("/workspaces/{workspace}/semantic-models")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@apigen.cli(#{ command: #["semantic-models", "list"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }], output: #{ mode: "raw" } })
op listSemanticModels(@path workspace: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticModelListResponse> | CommonErrors;

@tag("BI")
@summary("Describe semantic model")
@route("/workspaces/{workspace}/semantic-models/{model}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "describe"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
op getSemanticModel(@path workspace: string, @path("model") modelName: string): OkJson<SemanticModelDescriptionResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model relationships")
@route("/workspaces/{workspace}/semantic-models/{model}/relationships")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
op listSemanticRelationships(@path workspace: string, @path("model") modelName: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticRelationshipListResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model sources")
@route("/workspaces/{workspace}/semantic-models/{model}/sources")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
op listSemanticSources(@path workspace: string, @path("model") modelName: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticSourceListResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model fields")
@route("/workspaces/{workspace}/semantic-models/{model}/fields")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "model-fields"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
op listSemanticModelFields(@path workspace: string, @path("model") modelName: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticFieldListResponse> | CommonErrors;

@tag("BI")
@summary("Query semantic model")
@route("/workspaces/{workspace}/semantic-models/{model}/query")
@post
@apigen.authz(#{ mode: "privilege", privilege: "QUERY_DATA" })
@extension("x-leapview-object-scope", "semantic-model")
@extension("x-leapview-response-trailers", #["X-Next-Cursor"])
@apigen.cli(#{ command: #["semantic-models", "model-query"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "query_semantic_model", effect: "read", tags: #["semantic-model", "query"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "model", description: "Workspace ID to query." }, #{ source: "body", name: "limit", default: 25 }] },
  output: #{ mode: "project", select: #[#{ source: "/columns" }, #{ source: "/rows", countAs: "count" }], cursor: #{ source: "/page/nextCursor" } },
})
op querySemanticModel(@path workspace: WorkspaceId, @path("model") modelName: string, @header accept?: string, @body body?: SemanticQueryRequest): RowsetResponse<SemanticQueryResponse> | CommonErrors;

@tag("BI")
@summary("Explain semantic model query")
@route("/workspaces/{workspace}/semantic-models/{model}/query/explain")
@post
@apigen.authz(#{ mode: "privilege", privilege: "QUERY_DATA" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "model-explain"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
op explainSemanticModelQuery(@path workspace: string, @path("model") modelName: string, @body body?: SemanticQueryRequest): OkJson<SemanticExplainResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model datasets")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "datasets"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }], output: #{ mode: "raw" } })
op listSemanticDatasets(@path workspace: string, @path("model") modelName: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticDatasetListResponse> | CommonErrors;

@tag("BI")
@summary("Describe semantic model dataset")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "dataset"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
op getSemanticDataset(@path workspace: string, @path("model") modelName: string, @path dataset: string): OkJson<SemanticDatasetResponse> | CommonErrors;

@tag("BI")
@summary("List semantic model dataset fields")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/fields")
@get
@apigen.authz(#{ mode: "privilege", privilege: "VIEW_ITEM" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "fields"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
op listSemanticFields(@path workspace: string, @path("model") modelName: string, @path dataset: string, @query limit?: ListLimit, @query pageToken?: PageToken): OkJson<SemanticFieldListResponse> | CommonErrors;

@tag("BI")
@summary("Preview semantic model dataset rows")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview")
@post
@apigen.authz(#{ mode: "privilege", privilege: "PREVIEW_DATA" })
@extension("x-leapview-object-scope", "semantic-model")
@extension("x-leapview-response-trailers", #["X-Next-Cursor"])
@apigen.cli(#{ command: #["semantic-models", "preview"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
op previewSemanticDataset(@path workspace: string, @path("model") modelName: string, @path dataset: string, @header accept?: string, @body body?: SemanticPreviewRequest): RowsetResponse<SemanticQueryResponse> | CommonErrors;

@tag("BI")
@summary("Explain semantic model dataset row preview")
@route("/workspaces/{workspace}/semantic-models/{model}/datasets/{dataset}/preview/explain")
@post
@apigen.authz(#{ mode: "privilege", privilege: "PREVIEW_DATA" })
@extension("x-leapview-object-scope", "semantic-model")
@apigen.cli(#{ command: #["semantic-models", "explain-preview"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "model", displayName: "model" }, #{ source: "path", name: "dataset", displayName: "dataset" }], output: #{ mode: "raw" } })
op explainSemanticPreview(@path workspace: string, @path("model") modelName: string, @path dataset: string, @body body?: SemanticPreviewRequest): OkJson<SemanticExplainResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard page")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/query")
@post
@apigen.authz(#{ mode: "privilege", privilege: "QUERY_DATA" })
@extension("x-leapview-object-scope", "dashboard")
@apigen.cli(#{ command: #["dashboards", "query-page"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }], output: #{ mode: "raw" } })
op queryDashboardPage(@path workspace: string, @path dashboard: string, @path page: string, @body body?: DashboardPageQueryRequest): OkJson<DashboardPageQueryResponse> | CommonErrors;

@tag("BI")
@summary("Query dashboard visual")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/visuals/{visual}/query")
@post
@apigen.authz(#{ mode: "privilege", privilege: "QUERY_DATA" })
@extension("x-leapview-object-scope", "dashboard")
@extension("x-leapview-response-trailers", #["X-Next-Cursor"])
@apigen.cli(#{ command: #["dashboards", "visual-data"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "visual", displayName: "visual" }], output: #{ mode: "raw" } })
@apigen.tool(#{
  name: "query_dashboard_visual", effect: "read", tags: #["dashboard", "visual", "query"],
  input: #{ fields: #[#{ source: "path", name: "workspace", mode: "model", description: "Workspace ID to query." }] },
  output: #{ mode: "raw" },
})
op queryDashboardVisualData(@path workspace: WorkspaceId, @path dashboard: string, @path page: string, @path visual: string, @header accept?: string, @body body?: DashboardVisualQueryRequest): RowsetResponse<LeapViewVisualization.VisualizationEnvelope> | CommonErrors;

@tag("BI")
@summary("List dashboard filter values")
@route("/workspaces/{workspace}/dashboards/{dashboard}/pages/{page}/filters/{filter}/values")
@post
@apigen.authz(#{ mode: "privilege", privilege: "QUERY_DATA" })
@extension("x-leapview-object-scope", "dashboard")
@apigen.cli(#{ command: #["dashboards", "filter-options"], args: #[#{ source: "path", name: "workspace", displayName: "workspace" }, #{ source: "path", name: "dashboard", displayName: "dashboard" }, #{ source: "path", name: "page", displayName: "page" }, #{ source: "path", name: "filter", displayName: "filter" }], output: #{ mode: "raw" } })
op listDashboardFilterValues(@path workspace: string, @path dashboard: string, @path page: string, @path filter: string, @query limit?: ListLimit, @query pageToken?: PageToken, @body body?: DashboardPageQueryRequest): OkJson<DashboardFilterOptionListResponse> | CommonErrors;
