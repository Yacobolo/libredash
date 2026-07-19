package compiler

type catalogPayloadV1 struct {
	Workspace      catalogWorkspacePayloadV1   `json:"Workspace"`
	SemanticModels []catalogModelPayloadV1     `json:"SemanticModels"`
	Dashboards     []catalogDashboardPayloadV1 `json:"Dashboards"`
}

type catalogWorkspacePayloadV1 struct {
	ID          string `json:"ID"`
	Title       string `json:"Title"`
	Description string `json:"Description"`
}

type catalogModelPayloadV1 struct {
	ID          string `json:"ID"`
	Title       string `json:"Title"`
	Path        string `json:"Path"`
	Description string `json:"Description"`
}

type catalogDashboardPayloadV1 struct {
	ID          string   `json:"ID"`
	Title       string   `json:"Title"`
	Path        string   `json:"Path"`
	Description string   `json:"Description"`
	Tags        []string `json:"Tags"`
}

type connectionPayloadV1 struct {
	Kind                  string                      `json:"Kind"`
	Path                  string                      `json:"Path"`
	Root                  string                      `json:"Root"`
	Scope                 string                      `json:"Scope"`
	Host                  string                      `json:"Host"`
	Port                  int                         `json:"Port"`
	Database              string                      `json:"Database"`
	Username              string                      `json:"Username"`
	SSLMode               string                      `json:"SSLMode"`
	Options               map[string]any              `json:"Options"`
	Defaults              connectionDefaultsPayloadV1 `json:"Defaults"`
	CredentialsConfigured bool                        `json:"credentials_configured"`
}

type connectionDefaultsPayloadV1 struct {
	Options map[string]any `json:"Options"`
}

type sourcePayloadV1 struct {
	Format     string                          `json:"Format"`
	Path       string                          `json:"Path"`
	Connection string                          `json:"Connection"`
	Object     string                          `json:"Object"`
	Options    map[string]any                  `json:"Options"`
	Fields     map[string]sourceFieldPayloadV1 `json:"Fields"`
	Schema     schemaPayloadV1                 `json:"Schema"`
}

type sourceFieldPayloadV1 struct {
	Name        string `json:"Name"`
	Field       string `json:"Field"`
	Table       string `json:"Table"`
	Type        string `json:"Type"`
	Description string `json:"Description"`
}

type modelTablePayloadV1 struct {
	Source             string                          `json:"Source"`
	Sources            []string                        `json:"Sources"`
	SourceDependencies []string                        `json:"SourceDependencies"`
	ModelDependencies  []string                        `json:"ModelDependencies"`
	Transform          transformPayloadV1              `json:"Transform"`
	SQL                string                          `json:"SQL"`
	PrimaryKey         string                          `json:"PrimaryKey"`
	Grain              string                          `json:"Grain"`
	Dimensions         map[string]fieldPayloadV1       `json:"Dimensions"`
	Fields             map[string]fieldPayloadV1       `json:"Fields"`
	Columns            map[string]modelColumnPayloadV1 `json:"Columns"`
	Schema             schemaPayloadV1                 `json:"Schema"`
}

type semanticTablePayloadV1 struct {
	Table string `json:"Table"`
	modelTablePayloadV1
}

type transformPayloadV1 struct {
	SQL string `json:"SQL"`
}

type semanticModelPayloadV1 struct {
	Name          string                                `json:"Name"`
	Title         string                                `json:"Title"`
	Description   string                                `json:"Description"`
	Connections   map[string]connectionPayloadV1        `json:"Connections"`
	Sources       map[string]sourcePayloadV1            `json:"Sources"`
	Tables        map[string]modelTablePayloadV1        `json:"Tables"`
	Models        map[string]modelTablePayloadV1        `json:"Models"`
	Measures      map[string]measurePayloadV1           `json:"Measures"`
	Dimensions    map[string]semanticDimensionPayloadV1 `json:"Dimensions"`
	Metrics       map[string]metricPayloadV1            `json:"Metrics"`
	Relationships []relationshipPayloadV1               `json:"Relationships"`
}

type fieldPayloadV1 struct {
	Field       string `json:"Field"`
	Table       string `json:"Table"`
	Name        string `json:"Name"`
	Label       string `json:"Label"`
	Description string `json:"Description"`
	Type        string `json:"Type"`
	Expr        string `json:"Expr"`
	Expression  string `json:"Expression"`
}

type measurePayloadV1 struct {
	Field           string `json:"Field"`
	Fact            string `json:"Fact"`
	Name            string `json:"Name"`
	Label           string `json:"Label"`
	Description     string `json:"Description"`
	Aggregation     string `json:"Aggregation"`
	InputField      string `json:"InputField"`
	InputExpression string `json:"InputExpression"`
	Empty           string `json:"Empty"`
	Unit            string `json:"Unit"`
	Format          string `json:"Format"`
	Hidden          bool   `json:"Hidden"`
}

type semanticDimensionPayloadV1 struct {
	Name        string                                       `json:"Name"`
	Label       string                                       `json:"Label"`
	Description string                                       `json:"Description"`
	Type        string                                       `json:"Type"`
	Grains      []string                                     `json:"Grains"`
	Bindings    map[string]semanticDimensionBindingPayloadV1 `json:"Bindings"`
}

type semanticDimensionBindingPayloadV1 struct {
	Field string   `json:"Field"`
	Path  []string `json:"Path"`
}

type metricPayloadV1 struct {
	Name        string `json:"Name"`
	Label       string `json:"Label"`
	Description string `json:"Description"`
	Expression  string `json:"Expression"`
	Unit        string `json:"Unit"`
	Format      string `json:"Format"`
	Hidden      bool   `json:"Hidden"`
}

type relationshipPayloadV1 struct {
	ID          string `json:"ID"`
	Description string `json:"Description"`
	From        string `json:"From"`
	To          string `json:"To"`
	Cardinality string `json:"Cardinality"`
}

type modelColumnPayloadV1 struct {
	Field       string `json:"Field"`
	Name        string `json:"Name"`
	SourceField string `json:"SourceField"`
	Description string `json:"Description"`
	Type        string `json:"Type"`
}

type schemaPayloadV1 struct {
	Columns []schemaColumnPayloadV1 `json:"Columns"`
}

type schemaColumnPayloadV1 struct {
	Name         string `json:"Name"`
	Ordinal      int    `json:"Ordinal"`
	PhysicalType string `json:"PhysicalType"`
	Nullable     *bool  `json:"Nullable"`
	Default      string `json:"Default"`
	Comment      string `json:"Comment"`
	PrimaryKey   bool   `json:"PrimaryKey"`
}

type dashboardPayloadV1 struct {
	ID            string   `json:"ID"`
	Title         string   `json:"Title"`
	Description   string   `json:"Description"`
	SemanticModel string   `json:"SemanticModel"`
	Tags          []string `json:"Tags"`
}

type filterPayloadV1 struct {
	Type             string                  `json:"Type"`
	Label            string                  `json:"Label"`
	Description      string                  `json:"Description"`
	Dimension        string                  `json:"Dimension"`
	Fact             string                  `json:"Fact"`
	Default          any                     `json:"Default"`
	Custom           bool                    `json:"Custom"`
	Presets          []filterPresetPayloadV1 `json:"Presets"`
	Operator         string                  `json:"Operator"`
	Values           filterValuesPayloadV1   `json:"Values"`
	DefaultOperator  string                  `json:"DefaultOperator"`
	Operators        []string                `json:"Operators"`
	Options          []filterOptionPayloadV1 `json:"Options"`
	URLParam         string                  `json:"URLParam"`
	FromURLParam     string                  `json:"FromURLParam"`
	ToURLParam       string                  `json:"ToURLParam"`
	OperatorURLParam string                  `json:"OperatorURLParam"`
	Targets          filterTargetsPayloadV1  `json:"Targets"`
}

type filterPresetPayloadV1 struct {
	Value        string `json:"value"`
	Label        string `json:"label"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	RelativeDays int    `json:"relativeDays,omitempty"`
}

type filterValuesPayloadV1 struct {
	Source string `json:"source,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type filterOptionPayloadV1 struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type filterTargetsPayloadV1 struct {
	Visuals []string `json:"visuals,omitempty"`
	Tables  []string `json:"tables,omitempty"`
}

type visualPayloadV1 struct {
	Title           string               `json:"Title"`
	Description     string               `json:"Description"`
	Kind            string               `json:"Kind"`
	Shape           string               `json:"Shape"`
	Renderer        string               `json:"Renderer"`
	Type            string               `json:"Type"`
	Query           visualQueryPayloadV1 `json:"Query"`
	Options         map[string]any       `json:"Options"`
	RendererOptions map[string]any       `json:"RendererOptions"`
	Encode          map[string]string    `json:"Encode"`
	Interaction     selectionPayloadV1   `json:"Interaction"`
}

type selectionPayloadV1 struct {
	Toggle   bool                        `json:"Toggle"`
	Mappings []selectionMappingPayloadV1 `json:"Mappings"`
	Targets  []string                    `json:"Targets"`
}

type selectionMappingPayloadV1 struct {
	Field string `json:"Field"`
	Fact  string `json:"Fact"`
	Grain string `json:"Grain"`
	Value string `json:"Value"`
	Label string `json:"Label"`
}

type visualQueryPayloadV1 struct {
	Table      string             `json:"Table"`
	Dimensions []string           `json:"Dimensions"`
	Series     string             `json:"Series"`
	Measures   []string           `json:"Measures"`
	Time       queryTimePayloadV1 `json:"Time"`
	Sort       []sortPayloadV1    `json:"Sort"`
	Limit      int                `json:"Limit"`
}

type queryTimePayloadV1 struct {
	Field string `json:"field"`
	Grain string `json:"grain"`
	Alias string `json:"alias,omitempty"`
}

type sortPayloadV1 struct {
	Field     string `json:"Field"`
	Direction string `json:"Direction"`
	Expr      string `json:"Expr"`
}

type tablePayloadV1 struct {
	Title       string              `json:"Title"`
	Description string              `json:"Description"`
	Kind        string              `json:"Kind"`
	Query       tableQueryPayloadV1 `json:"Query"`
	Rows        []string            `json:"Rows"`
	ColumnDims  []string            `json:"ColumnDims"`
	DataColumns []fieldRefPayloadV1 `json:"DataColumns"`
	Style       tableStylePayloadV1 `json:"Style"`
	DefaultSort tableSortPayloadV1  `json:"DefaultSort"`
	Interaction selectionPayloadV1  `json:"Interaction"`
}

type tableQueryPayloadV1 struct {
	Table    string   `json:"Table"`
	Measures []string `json:"Measures"`
}

type fieldRefPayloadV1 struct {
	Field string `json:"field"`
	Alias string `json:"alias,omitempty"`
}

type tableStylePayloadV1 struct {
	Density string `json:"density"`
	Zebra   *bool  `json:"zebra"`
	Grid    string `json:"grid"`
}

type tableSortPayloadV1 struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type pagePayloadV1 struct {
	ID          string       `json:"ID"`
	Title       string       `json:"Title"`
	Description string       `json:"Description"`
	Canvas      pageCanvasV1 `json:"Canvas"`
	Grid        pageGridV1   `json:"Grid"`
}

type workspaceGroupPayloadV1 struct {
	ID          string                          `json:"ID"`
	Name        string                          `json:"Name"`
	Description string                          `json:"Description"`
	Members     []workspaceGroupMemberPayloadV1 `json:"Members"`
}

type workspaceGroupMemberPayloadV1 struct {
	PrincipalID string `json:"PrincipalID"`
	Email       string `json:"Email"`
	DisplayName string `json:"DisplayName"`
}

type workspaceRoleBindingPayloadV1 struct {
	ID      string                               `json:"ID"`
	Name    string                               `json:"Name"`
	Role    string                               `json:"Role"`
	Subject workspaceRoleBindingSubjectPayloadV1 `json:"Subject"`
}

type workspaceRoleBindingSubjectPayloadV1 struct {
	Kind        string `json:"Kind"`
	PrincipalID string `json:"PrincipalID"`
	Email       string `json:"Email"`
	DisplayName string `json:"DisplayName"`
	Group       string `json:"Group"`
}

type workspaceAgentPolicyPayloadV1 struct {
	ID           string                             `json:"ID"`
	Name         string                             `json:"Name"`
	Enabled      bool                               `json:"Enabled"`
	Tools        workspaceAgentPolicyToolsPayloadV1 `json:"Tools"`
	Instructions string                             `json:"Instructions"`
}

type workspaceAgentPolicyToolsPayloadV1 struct {
	Allow []string `json:"Allow"`
	Deny  []string `json:"Deny"`
}

type refreshPipelinePayloadV1 struct {
	ID            string                             `json:"ID"`
	Name          string                             `json:"Name"`
	SemanticModel string                             `json:"SemanticModel"`
	Schedules     []refreshPipelineSchedulePayloadV1 `json:"Schedules"`
}

type refreshPipelineSchedulePayloadV1 struct {
	Cron     string `json:"Cron"`
	Timezone string `json:"Timezone"`
}

type pageItemPayloadV1 struct {
	ID          string          `json:"ID"`
	Kind        string          `json:"Kind"`
	Visual      string          `json:"Visual"`
	Table       string          `json:"Table"`
	Filter      string          `json:"Filter"`
	Description string          `json:"Description"`
	Placement   pagePlacementV1 `json:"Placement"`
	Title       string          `json:"Title"`
	Subtitle    string          `json:"Subtitle"`
	Badges      []string        `json:"Badges"`
}

type pageCanvasV1 struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type pageGridV1 struct {
	Columns   int `json:"columns"`
	RowHeight int `json:"rowHeight"`
	Gap       int `json:"gap"`
	Padding   int `json:"padding"`
}

type pagePlacementV1 struct {
	Col     int `json:"col"`
	Row     int `json:"row"`
	ColSpan int `json:"colSpan"`
	RowSpan int `json:"rowSpan"`
}
