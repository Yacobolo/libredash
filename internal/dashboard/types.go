package dashboard

type Signals struct {
	Filters            Filters            `json:"filters"`
	Runtime            Runtime            `json:"runtime"`
	TableCommand       TableRequest       `json:"tableCommand"`
	InteractionCommand InteractionCommand `json:"interactionCommand"`
}

type Catalog struct {
	Workspace  CatalogWorkspace   `json:"workspace"`
	Models     []CatalogModel     `json:"models"`
	Dashboards []CatalogDashboard `json:"dashboards"`
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

type CatalogDashboard struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	SemanticModel string   `json:"semanticModel"`
	Tags          []string `json:"tags"`
	PageCount     int      `json:"pageCount"`
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
	ID          string        `json:"id" yaml:"id"`
	Kind        string        `json:"kind" yaml:"kind"`
	Visual      string        `json:"visual,omitempty" yaml:"visual"`
	Table       string        `json:"table,omitempty" yaml:"table"`
	Filter      string        `json:"filter,omitempty" yaml:"filter"`
	Description string        `json:"description,omitempty" yaml:"description"`
	Placement   PagePlacement `json:"placement" yaml:"placement"`
	X           float64       `json:"x" yaml:"-"`
	Y           float64       `json:"y" yaml:"-"`
	Width       float64       `json:"width" yaml:"-"`
	Height      float64       `json:"height" yaml:"-"`
	Eyebrow     string        `json:"eyebrow,omitempty" yaml:"eyebrow"`
	Title       string        `json:"title,omitempty" yaml:"title"`
	Subtitle    string        `json:"subtitle,omitempty" yaml:"subtitle"`
	Badges      []string      `json:"badges,omitempty" yaml:"badges"`
}

type Filters struct {
	Controls   map[string]FilterControl `json:"controls"`
	Selections []InteractionSelection   `json:"selections"`
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
	if f.Selections == nil {
		f.Selections = []InteractionSelection{}
	}
	return f
}

type InteractionSelection struct {
	ID              string                      `json:"id"`
	SourceKind      string                      `json:"sourceKind"`
	SourceID        string                      `json:"sourceId"`
	InteractionKind string                      `json:"interactionKind"`
	Entries         []InteractionSelectionEntry `json:"entries"`
	Label           string                      `json:"label"`
	Order           int                         `json:"order"`
}

type InteractionSelectionEntry struct {
	Mappings []InteractionSelectionMapping `json:"mappings"`
	Label    string                        `json:"label,omitempty"`
}

type InteractionSelectionMapping struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

type InteractionCommand struct {
	SourceKind      string                      `json:"sourceKind"`
	SourceID        string                      `json:"sourceId"`
	InteractionKind string                      `json:"interactionKind"`
	Action          string                      `json:"action"`
	Toggle          bool                        `json:"toggle"`
	Mappings        []InteractionCommandMapping `json:"mappings"`
}

const UIRowSelectionField = "__libredash.rowKey"

type InteractionCommandMapping struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

func (c InteractionCommand) IsEmpty() bool {
	if c.SourceKind == "" || c.SourceID == "" || c.InteractionKind == "" {
		return true
	}
	if c.Action == "clear" {
		return false
	}
	if len(c.Mappings) == 0 {
		return true
	}
	for _, mapping := range c.Mappings {
		if mapping.Field == "" || mapping.Value == "" {
			return true
		}
	}
	return false
}

func (f Filters) ApplyInteraction(command InteractionCommand) Filters {
	f = f.WithDefaults()
	if command.IsEmpty() {
		return f
	}

	selectionID := command.SourceKind + ":" + command.SourceID + ":" + command.InteractionKind
	next := make([]InteractionSelection, 0, len(f.Selections)+1)
	maxOrder := 0
	changed := false

	for _, selection := range f.Selections {
		if selection.Order > maxOrder {
			maxOrder = selection.Order
		}
		if selection.ID == selectionID || (selection.SourceKind == command.SourceKind && selection.SourceID == command.SourceID && selection.InteractionKind == command.InteractionKind) {
			changed = true
			if command.Action == "clear" {
				continue
			}
			selection.ID = selectionID
			if command.Action == "replace" {
				selection.Entries = updateSelectionEntries(nil, command.Mappings, false)
			} else {
				selection.Entries = updateSelectionEntries(selection.Entries, command.Mappings, command.Toggle)
			}
			selection.Label = interactionSelectionLabel(selection.Entries)
			if len(selection.Entries) > 0 {
				next = append(next, selection)
			}
			continue
		}
		next = append(next, selection)
	}

	if !changed && command.Action != "clear" {
		entries := updateSelectionEntries(nil, command.Mappings, false)
		if len(entries) > 0 {
			next = append(next, InteractionSelection{
				ID:              selectionID,
				SourceKind:      command.SourceKind,
				SourceID:        command.SourceID,
				InteractionKind: command.InteractionKind,
				Entries:         entries,
				Label:           interactionSelectionLabel(entries),
				Order:           maxOrder + 1,
			})
		}
	}

	f.Selections = next
	return f
}

func updateSelectionEntries(existing []InteractionSelectionEntry, incoming []InteractionCommandMapping, toggle bool) []InteractionSelectionEntry {
	entry := interactionSelectionEntry(incoming)
	if len(entry.Mappings) == 0 {
		return nil
	}

	out := make([]InteractionSelectionEntry, 0, len(existing)+1)
	found := false
	for _, existingEntry := range existing {
		if selectionEntriesEqual(existingEntry, entry) {
			found = true
			if toggle {
				continue
			}
		}
		out = append(out, copySelectionEntry(existingEntry))
	}
	if !found {
		out = append(out, entry)
	}
	return out
}

func interactionSelectionEntry(incoming []InteractionCommandMapping) InteractionSelectionEntry {
	mappings := make([]InteractionSelectionMapping, 0, len(incoming))
	for _, mapping := range incoming {
		mappings = append(mappings, InteractionSelectionMapping{
			Field: mapping.Field,
			Value: mapping.Value,
			Label: defaultString(mapping.Label, mapping.Value),
		})
	}
	entry := InteractionSelectionEntry{Mappings: mappings}
	entry.Label = interactionEntryLabel(entry)
	return entry
}

func selectionEntriesContain(existing []InteractionSelectionEntry, incoming InteractionSelectionEntry) bool {
	for _, entry := range existing {
		if selectionEntriesEqual(entry, incoming) {
			return true
		}
	}
	return false
}

func selectionEntriesEqual(left, right InteractionSelectionEntry) bool {
	if len(left.Mappings) != len(right.Mappings) {
		return false
	}
	values := make(map[string]int, len(left.Mappings))
	for _, mapping := range left.Mappings {
		values[selectionMappingKey(mapping)]++
	}
	for _, mapping := range right.Mappings {
		key := selectionMappingKey(mapping)
		if values[key] == 0 {
			return false
		}
		values[key]--
	}
	return true
}

func selectionMappingKey(mapping InteractionSelectionMapping) string {
	return mapping.Field + "\x00" + mapping.Value
}

func copySelectionEntry(entry InteractionSelectionEntry) InteractionSelectionEntry {
	out := InteractionSelectionEntry{
		Mappings: make([]InteractionSelectionMapping, len(entry.Mappings)),
		Label:    entry.Label,
	}
	copy(out.Mappings, entry.Mappings)
	return out
}

func interactionSelectionLabel(entries []InteractionSelectionEntry) string {
	if len(entries) == 0 {
		return ""
	}
	labels := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := entry.Label
		if label == "" {
			label = interactionEntryLabel(entry)
		}
		labels = append(labels, label)
	}
	return joinValues(labels)
}

func interactionEntryLabel(entry InteractionSelectionEntry) string {
	if len(entry.Mappings) == 0 {
		return ""
	}
	labels := make([]string, 0, len(entry.Mappings))
	for _, mapping := range entry.Mappings {
		label := mapping.Label
		if label == "" {
			label = selectionLabel(mapping.Field, []string{mapping.Value})
		}
		labels = append(labels, label)
	}
	return joinValues(labels)
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
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
	SetupRequired bool   `json:"setupRequired"`
}

type Visual struct {
	Version         int                         `json:"version"`
	ID              string                      `json:"id"`
	Kind            string                      `json:"kind"`
	Shape           string                      `json:"shape"`
	Renderer        string                      `json:"renderer"`
	Type            string                      `json:"type"`
	Title           string                      `json:"title"`
	Unit            string                      `json:"unit"`
	Format          string                      `json:"format,omitempty"`
	Interaction     InteractionConfig           `json:"interaction"`
	Dimensions      []string                    `json:"dimensions"`
	Measure         string                      `json:"measure"`
	Measures        []string                    `json:"measures"`
	Series          []string                    `json:"series"`
	Options         map[string]any              `json:"options"`
	RendererOptions map[string]map[string]any   `json:"rendererOptions"`
	Selection       []InteractionSelectionEntry `json:"selection"`
	Data            []Datum                     `json:"data"`
}

type Datum map[string]any

type InteractionConfig struct {
	Kind     string                     `json:"kind"`
	Toggle   bool                       `json:"toggle"`
	Mappings []InteractionConfigMapping `json:"mappings"`
	Targets  []string                   `json:"targets,omitempty"`
}

type InteractionConfigMapping struct {
	Field string `json:"field"`
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

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
	Version       int                         `json:"version"`
	Kind          string                      `json:"kind"`
	Title         string                      `json:"title"`
	Style         TableStyle                  `json:"style"`
	Interaction   InteractionConfig           `json:"interaction"`
	Selection     []InteractionSelectionEntry `json:"selection"`
	Columns       []TableColumn               `json:"columns"`
	TotalRows     int                         `json:"totalRows"`
	AvailableRows int                         `json:"availableRows"`
	IsCapped      bool                        `json:"isCapped"`
	RowCap        int                         `json:"rowCap"`
	ChunkSize     int                         `json:"chunkSize"`
	RowHeight     int                         `json:"rowHeight"`
	ResetVersion  int                         `json:"resetVersion"`
	Sort          TableSort                   `json:"sort"`
	Blocks        map[string]TableBlock       `json:"blocks"`
	LoadingBlock  string                      `json:"loadingBlock"`
	Error         string                      `json:"error"`
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
		Selection:     []InteractionSelectionEntry{},
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

func EmptyPatch(filters Filters, err error) Patch {
	message := ""
	if err != nil {
		message = err.Error()
	}

	return Patch{
		Filters: filters.WithDefaults(),
		Status: Status{
			Loading:       false,
			Error:         message,
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
		Dimensions:      []string{dimension},
		Measure:         measure,
		Measures:        []string{measure},
		Series:          []string{},
		Options:         map[string]any{},
		RendererOptions: map[string]map[string]any{},
		Selection:       []InteractionSelectionEntry{},
		Data:            []Datum{},
	}
}
