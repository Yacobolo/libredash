package api

type DashboardSummary struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	SemanticModel string   `json:"semanticModel"`
	Tags          []string `json:"tags"`
	PageCount     int      `json:"pageCount"`
}

type DashboardListResponse struct {
	Items []DashboardSummary `json:"items"`
	Page  PageInfo           `json:"page"`
}

type SemanticModelSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SemanticModelListResponse struct {
	Items []SemanticModelSummary `json:"items"`
	Page  PageInfo               `json:"page"`
}

type ModelRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type DashboardManifestResponse struct {
	ID            string                  `json:"id"`
	Title         string                  `json:"title"`
	Description   string                  `json:"description,omitempty"`
	SemanticModel string                  `json:"semantic_model,omitempty"`
	Model         *ModelRef               `json:"model,omitempty"`
	Counts        DashboardManifestCounts `json:"counts"`
	Pages         []DashboardManifestPage `json:"pages"`
	DetailTools   map[string]string       `json:"detail_tools"`
}

type DashboardManifestCounts struct {
	Pages   int `json:"pages"`
	Visuals int `json:"visuals"`
	Filters int `json:"filters"`
}

type DashboardManifestPage struct {
	ID          string                       `json:"id"`
	Title       string                       `json:"title"`
	Description string                       `json:"description,omitempty"`
	Components  []DashboardManifestComponent `json:"components"`
}

type DashboardManifestComponent struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Title string `json:"title,omitempty"`
}

type DashboardComponentPlacement struct {
	Col     int `json:"col,omitempty"`
	Row     int `json:"row,omitempty"`
	ColSpan int `json:"colSpan,omitempty"`
	RowSpan int `json:"rowSpan,omitempty"`
}

type DashboardComponentResponse struct {
	ID          string                       `json:"id"`
	Kind        string                       `json:"kind"`
	Ref         string                       `json:"ref,omitempty"`
	Title       string                       `json:"title,omitempty"`
	Description string                       `json:"description,omitempty"`
	Placement   *DashboardComponentPlacement `json:"placement,omitempty"`
	X           float64                      `json:"x,omitempty"`
	Y           float64                      `json:"y,omitempty"`
	Width       float64                      `json:"width,omitempty"`
	Height      float64                      `json:"height,omitempty"`
	VisualID    string                       `json:"visualId,omitempty"`
	FilterID    string                       `json:"filterId,omitempty"`
}

type DashboardComponentListResponse struct {
	Items []DashboardComponentResponse `json:"items"`
	Page  PageInfo                     `json:"page"`
}

type DashboardPageResponse struct {
	ID          string                       `json:"id"`
	Title       string                       `json:"title"`
	Description string                       `json:"description,omitempty"`
	Components  []DashboardComponentResponse `json:"components"`
}

type DashboardTableColumn struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Role   string `json:"role,omitempty"`
	Format string `json:"format,omitempty"`
}

type DashboardFilterDescribeResponse struct {
	ID          string                       `json:"id"`
	ComponentID string                       `json:"componentId,omitempty"`
	Title       string                       `json:"title,omitempty"`
	Description string                       `json:"description,omitempty"`
	Field       string                       `json:"field,omitempty"`
	MultiSelect bool                         `json:"multiSelect"`
	Placement   *DashboardComponentPlacement `json:"placement,omitempty"`
}

type DashboardVisualDescribeResponse struct {
	ID          string                       `json:"id"`
	ComponentID string                       `json:"componentId,omitempty"`
	Shape       string                       `json:"shape,omitempty"`
	Renderer    string                       `json:"renderer,omitempty"`
	Type        string                       `json:"type,omitempty"`
	Title       string                       `json:"title,omitempty"`
	Description string                       `json:"description,omitempty"`
	Query       map[string]any               `json:"query,omitempty"`
	Extensions  map[string]map[string]any    `json:"extensions,omitempty"`
	Interaction map[string]any               `json:"interaction,omitempty"`
	Columns     []DashboardTableColumn       `json:"columns,omitempty"`
	Cardinality string                       `json:"cardinality,omitempty"`
	Placement   *DashboardComponentPlacement `json:"placement,omitempty"`
	X           float64                      `json:"x,omitempty"`
	Y           float64                      `json:"y,omitempty"`
	Width       float64                      `json:"width,omitempty"`
	Height      float64                      `json:"height,omitempty"`
}

type SemanticModelDescriptionResponse struct {
	ID          string                      `json:"id"`
	Title       string                      `json:"title"`
	Description string                      `json:"description"`
	Dashboards  []ModelDashboardUsage       `json:"dashboards"`
	Counts      *SemanticModelCounts        `json:"counts,omitempty"`
	Tables      []SemanticModelTableSummary `json:"tables,omitempty"`
}

type SemanticModelCounts struct {
	Sources             int `json:"sources"`
	ModelTables         int `json:"model_tables"`
	Fields              int `json:"fields"`
	Facts               int `json:"facts"`
	ConformedDimensions int `json:"conformed_dimensions"`
	AtomicMeasures      int `json:"atomic_measures"`
	Metrics             int `json:"metrics"`
	Relationships       int `json:"relationships"`
}

type SemanticModelTableSummary struct {
	ID          string   `json:"id"`
	Roles       []string `json:"roles"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Fields      int      `json:"fields"`
}

type SemanticDatasetSummary struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Source       string `json:"source"`
	Description  string `json:"description"`
	FieldCount   int    `json:"fieldCount"`
	MeasureCount int    `json:"measureCount"`
}

type SemanticDatasetListResponse struct {
	Items []SemanticDatasetSummary `json:"items"`
	Page  PageInfo                 `json:"page"`
}

type SemanticDatasetResponse struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Source       string   `json:"source"`
	Sources      []string `json:"sources"`
	Description  string   `json:"description"`
	PrimaryKey   string   `json:"primaryKey"`
	Grain        string   `json:"grain"`
	FieldCount   int      `json:"fieldCount"`
	MeasureCount int      `json:"measureCount"`
}

type SemanticFieldResponse struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Table       string   `json:"table"`
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Format      string   `json:"format,omitempty"`
	Grain       string   `json:"grain,omitempty"`
	Time        string   `json:"time,omitempty"`
	Grains      []string `json:"grains,omitempty"`
}

type SemanticFieldListResponse struct {
	Items []SemanticFieldResponse `json:"items"`
	Page  PageInfo                `json:"page"`
}

type SemanticRelationshipResponse struct {
	ID          string `json:"id"`
	FromDataset string `json:"fromDataset"`
	FromField   string `json:"fromField"`
	ToDataset   string `json:"toDataset"`
	ToField     string `json:"toField"`
	Cardinality string `json:"cardinality"`
	Active      bool   `json:"active"`
}

type SemanticRelationshipListResponse struct {
	Items []SemanticRelationshipResponse `json:"items"`
	Page  PageInfo                       `json:"page"`
}

type SemanticSourceResponse struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Connection  string `json:"connection,omitempty"`
	Table       string `json:"table,omitempty"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
}

type SemanticSourceListResponse struct {
	Items []SemanticSourceResponse `json:"items"`
	Page  PageInfo                 `json:"page"`
}

type SemanticFieldRef struct {
	Field string `json:"field"`
	Alias string `json:"alias,omitempty"`
}

type SemanticTimeRef struct {
	Field string `json:"field"`
	Grain string `json:"grain,omitempty"`
	Alias string `json:"alias,omitempty"`
}

type SemanticSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

type SemanticFilter struct {
	Field    string                `json:"field,omitempty"`
	Fact     string                `json:"fact,omitempty"`
	Operator string                `json:"operator,omitempty"`
	Values   []any                 `json:"values,omitempty"`
	Groups   []SemanticFilterGroup `json:"groups,omitempty"`
}

type SemanticFilterGroup struct {
	Filters []SemanticFilter `json:"filters"`
}

type SemanticQueryRequest struct {
	Dimensions []SemanticFieldRef `json:"dimensions,omitempty"`
	Measures   []SemanticFieldRef `json:"measures,omitempty"`
	Time       *SemanticTimeRef   `json:"time,omitempty"`
	Filters    []SemanticFilter   `json:"filters,omitempty"`
	Sort       []SemanticSort     `json:"sort,omitempty"`
	Limit      int                `json:"limit,omitempty"`
	PageToken  string             `json:"pageToken,omitempty"`
}

type SemanticPreviewRequest struct {
	Dimensions []SemanticFieldRef `json:"dimensions,omitempty"`
	Measures   []SemanticFieldRef `json:"measures,omitempty"`
	Filters    []SemanticFilter   `json:"filters,omitempty"`
	Sort       []SemanticSort     `json:"sort,omitempty"`
	Limit      int                `json:"limit,omitempty"`
	PageToken  string             `json:"pageToken,omitempty"`
}

type QueryColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

type SemanticQueryResponse struct {
	QueryID         string        `json:"queryId"`
	ServingSnapshot string        `json:"servingSnapshot"`
	Columns         []QueryColumn `json:"columns"`
	Rows            [][]string    `json:"rows"`
	Page            PageInfo      `json:"page"`
}

type SemanticExplainResponse struct {
	Mode                 string           `json:"mode"`
	Facts                []string         `json:"facts"`
	StitchDimensions     []string         `json:"stitchDimensions"`
	PhysicalDependencies []string         `json:"physicalDependencies"`
	RelationshipPaths    []string         `json:"relationshipPaths"`
	SQL                  string           `json:"sql"`
	Args                 []map[string]any `json:"args"`
	Columns              []string         `json:"columns"`
	Warnings             []string         `json:"warnings"`
}

type ModelDashboardUsage struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	SemanticModel string `json:"semantic_model"`
	Pages         int    `json:"pages"`
}

type DashboardPageQueryRequest struct {
	Filters map[string]any `json:"filters"`
}

type DashboardTableQueryRequest struct {
	PageID  string         `json:"pageId"`
	Count   int            `json:"count"`
	Filters map[string]any `json:"filters"`
}

type DashboardVisualQueryRequest struct {
	Limit     int            `json:"limit"`
	PageToken string         `json:"pageToken"`
	Filters   map[string]any `json:"filters"`
}

type DashboardTableQueryResponse struct {
	ID              string        `json:"id"`
	Type            string        `json:"type"`
	QueryID         string        `json:"queryId"`
	ServingSnapshot string        `json:"servingSnapshot"`
	Title           string        `json:"title"`
	Columns         []QueryColumn `json:"columns"`
	Rows            [][]string    `json:"rows"`
	AvailableRows   int           `json:"availableRows"`
	Page            PageInfo      `json:"page"`
}

type DashboardFilterOptionResponse struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type DashboardFilterOptionListResponse struct {
	Items []DashboardFilterOptionResponse `json:"items"`
	Page  PageInfo                        `json:"page"`
}
