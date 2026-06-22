package dashboard

type Signals struct {
	Filters       Filters       `json:"filters"`
	Runtime       Runtime       `json:"runtime"`
	TableCommand  TableRequest  `json:"tableCommand"`
	VisualCommand VisualCommand `json:"visualCommand"`
}

type Catalog struct {
	Workspace   CatalogWorkspace    `json:"workspace"`
	Models      []CatalogModel      `json:"models"`
	MetricViews []CatalogMetricView `json:"metricViews"`
	Dashboards  []CatalogDashboard  `json:"dashboards"`
}

type CatalogWorkspace struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CatalogModel struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type CatalogMetricView struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	SemanticModel string `json:"semanticModel"`
	ModelTitle    string `json:"modelTitle"`
}

type CatalogDashboard struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	MetricViews      []string `json:"metricViews"`
	MetricViewTitles []string `json:"metricViewTitles"`
	Tags             []string `json:"tags"`
	PageCount        int      `json:"pageCount"`
}

type Page struct {
	ID          string       `json:"id" yaml:"id"`
	Title       string       `json:"title" yaml:"title"`
	Description string       `json:"description,omitempty" yaml:"description"`
	Canvas      PageCanvas   `json:"canvas" yaml:"canvas"`
	Grid        PageGrid     `json:"grid" yaml:"grid"`
	Visuals     []PageVisual `json:"visuals" yaml:"visuals"`
	Width       int          `json:"width,omitempty" yaml:"-"`
	Height      int          `json:"height,omitempty" yaml:"-"`
}

type PageCanvas struct {
	Width  int `json:"width" yaml:"width"`
	Height int `json:"height" yaml:"height"`
}

type PageGrid struct {
	Columns   int `json:"columns" yaml:"columns"`
	RowHeight int `json:"rowHeight" yaml:"row_height"`
	Gap       int `json:"gap" yaml:"gap"`
	Padding   int `json:"padding" yaml:"padding"`
}

type PagePlacement struct {
	Col     int `json:"col" yaml:"col"`
	Row     int `json:"row" yaml:"row"`
	ColSpan int `json:"colSpan" yaml:"col_span"`
	RowSpan int `json:"rowSpan" yaml:"row_span"`
}

func (p Page) WithDefaults() Page {
	if p.Canvas.Width <= 0 {
		if p.Width > 0 {
			p.Canvas.Width = p.Width
		} else {
			p.Canvas.Width = 1366
		}
	}
	if p.Canvas.Height <= 0 {
		if p.Height > 0 {
			p.Canvas.Height = p.Height
		} else {
			p.Canvas.Height = 940
		}
	}
	if p.Grid.Columns <= 0 {
		p.Grid.Columns = 12
	}
	if p.Grid.RowHeight <= 0 {
		p.Grid.RowHeight = 48
	}
	if p.Grid.Gap < 0 {
		p.Grid.Gap = 0
	}
	if p.Grid.Gap == 0 {
		p.Grid.Gap = 16
	}
	if p.Grid.Padding < 0 {
		p.Grid.Padding = 0
	}
	p.Width = p.Canvas.Width
	p.Height = p.Canvas.Height
	return p
}

func (p Page) PlacedVisuals() []PageVisual {
	p = p.WithDefaults()
	visuals := make([]PageVisual, 0, len(p.Visuals))
	for _, visual := range p.Visuals {
		if visual.Placement.IsZero() {
			visuals = append(visuals, visual)
			continue
		}
		visual.X, visual.Y, visual.Width, visual.Height = p.Grid.Rect(p.Canvas, visual.Placement)
		visuals = append(visuals, visual)
	}
	return visuals
}

func (g PageGrid) Rect(canvas PageCanvas, placement PagePlacement) (float64, float64, float64, float64) {
	g = Page{Canvas: canvas, Grid: g}.WithDefaults().Grid
	availableWidth := float64(canvas.Width - (g.Padding * 2) - (g.Gap * (g.Columns - 1)))
	colWidth := availableWidth / float64(g.Columns)
	x := float64(g.Padding) + float64(placement.Col-1)*(colWidth+float64(g.Gap))
	y := float64(g.Padding) + float64(placement.Row-1)*float64(g.RowHeight+g.Gap)
	width := float64(placement.ColSpan)*colWidth + float64(placement.ColSpan-1)*float64(g.Gap)
	height := float64(placement.RowSpan*g.RowHeight) + float64((placement.RowSpan-1)*g.Gap)
	return x, y, width, height
}

func (p PagePlacement) IsZero() bool {
	return p.Col == 0 && p.Row == 0 && p.ColSpan == 0 && p.RowSpan == 0
}

type PageVisual struct {
	ID        string        `json:"id" yaml:"id"`
	Kind      string        `json:"kind" yaml:"kind"`
	Visual    string        `json:"visual,omitempty" yaml:"visual"`
	Table     string        `json:"table,omitempty" yaml:"table"`
	Filter    string        `json:"filter,omitempty" yaml:"filter"`
	Placement PagePlacement `json:"placement" yaml:"placement"`
	X         float64       `json:"x" yaml:"-"`
	Y         float64       `json:"y" yaml:"-"`
	Width     float64       `json:"width" yaml:"-"`
	Height    float64       `json:"height" yaml:"-"`
	Eyebrow   string        `json:"eyebrow,omitempty" yaml:"eyebrow"`
	Title     string        `json:"title,omitempty" yaml:"title"`
	Subtitle  string        `json:"subtitle,omitempty" yaml:"subtitle"`
	Badges    []string      `json:"badges,omitempty" yaml:"badges"`
}

type Filters struct {
	Controls         map[string]FilterControl `json:"controls"`
	VisualSelections []VisualSelection        `json:"visualSelections"`
}

type FilterControl struct {
	Type     string   `json:"type"`
	Operator string   `json:"operator,omitempty"`
	Preset   string   `json:"preset,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Value    string   `json:"value,omitempty"`
	Values   []string `json:"values,omitempty"`
}

type Runtime struct {
	ClientID    string `json:"clientId"`
	DashboardID string `json:"dashboardId"`
	PageID      string `json:"pageId"`
	ModelID     string `json:"modelId"`
}

func (f Filters) WithDefaults() Filters {
	if f.Controls == nil {
		f.Controls = map[string]FilterControl{}
	}
	if f.VisualSelections == nil {
		f.VisualSelections = []VisualSelection{}
	}
	return f
}

type VisualSelection struct {
	ID       string   `json:"id"`
	VisualID string   `json:"visualId"`
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
	Label    string   `json:"label"`
	Order    int      `json:"order"`
}

type VisualCommand struct {
	VisualID string `json:"visualId"`
	Field    string `json:"field"`
	Value    string `json:"value"`
	Label    string `json:"label"`
	Mode     string `json:"mode"`
}

func (c VisualCommand) IsEmpty() bool {
	return c.VisualID == "" || c.Field == "" || c.Value == ""
}

func (f Filters) ToggleSelection(command VisualCommand) Filters {
	f = f.WithDefaults()
	if command.IsEmpty() {
		return f
	}

	selectionID := command.VisualID + ":" + command.Field
	next := make([]VisualSelection, 0, len(f.VisualSelections)+1)
	toggled := false
	maxOrder := 0

	for _, selection := range f.VisualSelections {
		if selection.Order > maxOrder {
			maxOrder = selection.Order
		}
		if selection.ID == selectionID || (selection.VisualID == command.VisualID && selection.Field == command.Field) {
			values, removed := toggleValue(selection.Values, command.Value)
			if len(values) > 0 {
				selection.ID = selectionID
				selection.Operator = "in"
				selection.Values = values
				selection.Label = selectionLabel(command.Field, values)
				next = append(next, selection)
			}
			toggled = true
			if removed && command.Mode != "replace" {
				continue
			}
			continue
		}
		next = append(next, selection)
	}

	if !toggled {
		next = append(next, VisualSelection{
			ID:       selectionID,
			VisualID: command.VisualID,
			Field:    command.Field,
			Operator: "in",
			Values:   []string{command.Value},
			Label:    selectionLabel(command.Field, []string{command.Value}),
			Order:    maxOrder + 1,
		})
	}

	f.VisualSelections = next
	return f
}

func toggleValue(values []string, value string) ([]string, bool) {
	next := make([]string, 0, len(values)+1)
	removed := false
	for _, existing := range values {
		if existing == value {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	if !removed {
		next = append(next, value)
	}
	return next, removed
}

func selectionLabel(field string, values []string) string {
	if len(values) == 1 {
		return field + " is " + values[0]
	}
	return field + " in " + joinValues(values)
}

func joinValues(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += ", " + value
	}
	return out
}

type Patch struct {
	Filters       Filters                   `json:"filters"`
	FilterOptions map[string][]FilterOption `json:"filterOptions,omitempty"`
	Status        Status                    `json:"status"`
	Visuals       map[string]Visual         `json:"visuals"`
}

type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Status struct {
	Loading       bool   `json:"loading"`
	Error         string `json:"error"`
	LastUpdated   string `json:"lastUpdated"`
	DataDirectory string `json:"dataDirectory"`
	SetupRequired bool   `json:"setupRequired"`
}

type Visual struct {
	Version         int                       `json:"version"`
	ID              string                    `json:"id"`
	Kind            string                    `json:"kind"`
	Shape           string                    `json:"shape"`
	Renderer        string                    `json:"renderer"`
	Type            string                    `json:"type"`
	Title           string                    `json:"title"`
	Unit            string                    `json:"unit"`
	Format          string                    `json:"format,omitempty"`
	Field           string                    `json:"field"`
	Dimensions      []string                  `json:"dimensions"`
	Measure         string                    `json:"measure"`
	Measures        []string                  `json:"measures"`
	Series          []string                  `json:"series"`
	Options         map[string]any            `json:"options"`
	RendererOptions map[string]map[string]any `json:"rendererOptions"`
	Selection       []string                  `json:"selection"`
	Data            []Datum                   `json:"data"`
}

type Datum map[string]any

type TableRequest struct {
	Table        string    `json:"table"`
	Block        string    `json:"block"`
	Start        int       `json:"start"`
	Count        int       `json:"count"`
	RequestSeq   int       `json:"requestSeq"`
	Sort         TableSort `json:"sort"`
	ResetVersion int       `json:"resetVersion"`
}

const (
	TableChunkSize         = 50
	TableInteractiveRowCap = 10000
	TableRowHeight         = 34
	TableMaxRequestCount   = 1000
)

type TableStyle struct {
	Density string `json:"density" yaml:"density"`
	Zebra   *bool  `json:"zebra" yaml:"zebra"`
	Grid    string `json:"grid" yaml:"grid"`
}

func (s TableStyle) WithDefaults() TableStyle {
	if s.Density != "compact" && s.Density != "spacious" {
		s.Density = "comfortable"
	}
	if s.Zebra == nil {
		zebra := true
		s.Zebra = &zebra
	}
	switch s.Grid {
	case "none", "columns", "full":
	default:
		s.Grid = "rows"
	}
	return s
}

func (s TableStyle) RowHeight() int {
	switch s.WithDefaults().Density {
	case "compact":
		return 28
	case "spacious":
		return 42
	default:
		return TableRowHeight
	}
}

func DefaultTableRequest() TableRequest {
	return TableRequest{
		Table: "orders",
		Block: "all",
		Start: 0,
		Count: TableChunkSize,
		Sort:  TableSort{Key: "purchase_date", Direction: "desc"},
	}
}

func (r TableRequest) WithDefaults() TableRequest {
	defaults := DefaultTableRequest()
	if r.Table == "" {
		r.Table = defaults.Table
	}
	if r.Block == "" {
		r.Block = defaults.Block
	}
	if r.Block != "all" && r.Block != "a" && r.Block != "b" && r.Block != "c" {
		r.Block = defaults.Block
	}
	if r.Count <= 0 {
		r.Count = defaults.Count
	}
	if r.Count > TableMaxRequestCount {
		r.Count = TableMaxRequestCount
	}
	if r.Start < 0 {
		r.Start = 0
	}
	if r.RequestSeq < 0 {
		r.RequestSeq = 0
	}
	if r.Sort.Key == "" {
		r.Sort = defaults.Sort
	}
	if r.Sort.Direction != "asc" && r.Sort.Direction != "desc" {
		r.Sort.Direction = defaults.Sort.Direction
	}
	return r
}

func (r TableRequest) Reset() TableRequest {
	r = r.WithDefaults()
	r.Block = "all"
	r.Start = 0
	r.Count = TableChunkSize
	r.ResetVersion++
	return r
}

type TableSort struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type Table struct {
	Version       int                   `json:"version"`
	Kind          string                `json:"kind"`
	Title         string                `json:"title"`
	Style         TableStyle            `json:"style"`
	Columns       []TableColumn         `json:"columns"`
	TotalRows     int                   `json:"totalRows"`
	AvailableRows int                   `json:"availableRows"`
	IsCapped      bool                  `json:"isCapped"`
	RowCap        int                   `json:"rowCap"`
	ChunkSize     int                   `json:"chunkSize"`
	RowHeight     int                   `json:"rowHeight"`
	ResetVersion  int                   `json:"resetVersion"`
	Sort          TableSort             `json:"sort"`
	Blocks        map[string]TableBlock `json:"blocks"`
	LoadingBlock  string                `json:"loadingBlock"`
	Error         string                `json:"error"`
}

type TableBlock struct {
	Start        int              `json:"start"`
	RequestSeq   int              `json:"requestSeq"`
	ResetVersion int              `json:"resetVersion"`
	Sort         TableSort        `json:"sort"`
	Rows         []map[string]any `json:"rows"`
}

type TableColumn struct {
	Key         string                `json:"key" yaml:"key"`
	Label       string                `json:"label" yaml:"label"`
	Align       string                `json:"align,omitempty" yaml:"align,omitempty"`
	Role        string                `json:"role,omitempty" yaml:"role,omitempty"`
	Group       string                `json:"group,omitempty" yaml:"group,omitempty"`
	Measure     string                `json:"measure,omitempty" yaml:"measure,omitempty"`
	ColumnValue string                `json:"columnValue,omitempty" yaml:"column_value,omitempty"`
	Width       int                   `json:"width,omitempty" yaml:"width,omitempty"`
	Format      string                `json:"format,omitempty" yaml:"format,omitempty"`
	Formatting  []TableFormattingRule `json:"formatting,omitempty" yaml:"formatting,omitempty"`
}

type TableFormattingRule struct {
	Kind       string            `json:"kind" yaml:"kind"`
	Values     map[string]string `json:"values,omitempty" yaml:"values,omitempty"`
	Min        *float64          `json:"min,omitempty" yaml:"min,omitempty"`
	Max        *float64          `json:"max,omitempty" yaml:"max,omitempty"`
	Color      string            `json:"color,omitempty" yaml:"color,omitempty"`
	Background string            `json:"background,omitempty" yaml:"background,omitempty"`
	LowColor   string            `json:"lowColor,omitempty" yaml:"low_color,omitempty"`
	HighColor  string            `json:"highColor,omitempty" yaml:"high_color,omitempty"`
}

func OrdersTableColumns() []TableColumn {
	return []TableColumn{
		{Key: "order_id", Label: "Order"},
		{Key: "purchase_date", Label: "Purchased"},
		{Key: "status", Label: "Status"},
		{Key: "state", Label: "State"},
		{Key: "category", Label: "Category"},
		{Key: "revenue", Label: "Revenue", Align: "right"},
		{Key: "review_score", Label: "Review", Align: "right"},
		{Key: "delivery_days", Label: "Delivery", Align: "right"},
	}
}

func EmptyTable(request TableRequest, err error) Table {
	request = request.WithDefaults()
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Table{
		Version:       2,
		Kind:          "data_table",
		Title:         "Orders",
		Style:         TableStyle{}.WithDefaults(),
		Columns:       OrdersTableColumns(),
		TotalRows:     0,
		AvailableRows: 0,
		IsCapped:      false,
		RowCap:        TableInteractiveRowCap,
		ChunkSize:     TableChunkSize,
		RowHeight:     TableStyle{}.RowHeight(),
		ResetVersion:  request.ResetVersion,
		Sort:          request.Sort,
		Blocks:        emptyTableBlocks(),
		LoadingBlock:  "",
		Error:         message,
	}
}

func emptyTableBlocks() map[string]TableBlock {
	return map[string]TableBlock{
		"a": {Start: 0, Rows: []map[string]any{}},
		"b": {Start: TableChunkSize, Rows: []map[string]any{}},
		"c": {Start: TableChunkSize * 2, Rows: []map[string]any{}},
	}
}

func EmptyPatch(filters Filters, dataDir string, err error) Patch {
	message := ""
	if err != nil {
		message = err.Error()
	}

	return Patch{
		Filters: filters.WithDefaults(),
		Status: Status{
			Loading:       false,
			Error:         message,
			DataDirectory: dataDir,
			SetupRequired: err != nil,
		},
		Visuals: map[string]Visual{},
	}
}

func emptyChart(id, chartType, title, unit, dimension, measure string) Visual {
	return Visual{
		Version:         3,
		ID:              id,
		Kind:            "chart",
		Shape:           "category_value",
		Renderer:        "echarts",
		Type:            chartType,
		Title:           title,
		Unit:            unit,
		Field:           dimension,
		Dimensions:      []string{dimension},
		Measure:         measure,
		Measures:        []string{measure},
		Series:          []string{},
		Options:         map[string]any{},
		RendererOptions: map[string]map[string]any{},
		Selection:       []string{},
		Data:            []Datum{},
	}
}
