package dashboard

type Signals struct {
	Filters      Filters      `json:"filters"`
	Runtime      Runtime      `json:"runtime"`
	TableCommand TableRequest `json:"tableCommand"`
	ChartCommand ChartCommand `json:"chartCommand"`
}

type Page struct {
	ID          string       `json:"id" yaml:"id"`
	Title       string       `json:"title" yaml:"title"`
	Description string       `json:"description,omitempty" yaml:"description"`
	Width       int          `json:"width" yaml:"width"`
	Height      int          `json:"height" yaml:"height"`
	Visuals     []PageVisual `json:"visuals" yaml:"visuals"`
}

func (p Page) WithDefaults() Page {
	if p.Width <= 0 {
		p.Width = 1366
	}
	if p.Height <= 0 {
		p.Height = 940
	}
	return p
}

type PageVisual struct {
	ID       string   `json:"id" yaml:"id"`
	Kind     string   `json:"kind" yaml:"kind"`
	Visual   string   `json:"visual,omitempty" yaml:"visual"`
	Table    string   `json:"table,omitempty" yaml:"table"`
	X        int      `json:"x" yaml:"x"`
	Y        int      `json:"y" yaml:"y"`
	Width    int      `json:"width" yaml:"width"`
	Height   int      `json:"height" yaml:"height"`
	Eyebrow  string   `json:"eyebrow,omitempty" yaml:"eyebrow"`
	Title    string   `json:"title,omitempty" yaml:"title"`
	Subtitle string   `json:"subtitle,omitempty" yaml:"subtitle"`
	Badges   []string `json:"badges,omitempty" yaml:"badges"`
}

type ModelGraph struct {
	Name  string      `json:"name"`
	Title string      `json:"title"`
	Stats ModelStats  `json:"stats"`
	Nodes []ModelNode `json:"nodes"`
	Edges []ModelEdge `json:"edges"`
}

type ModelStats struct {
	Sources       int `json:"sources"`
	CacheTables   int `json:"cacheTables"`
	Metrics       int `json:"metrics"`
	Visuals       int `json:"visuals"`
	ReportTables  int `json:"reportTables"`
	Relationships int `json:"relationships"`
}

type ModelNode struct {
	ID          string       `json:"id"`
	Label       string       `json:"label"`
	Kind        string       `json:"kind"`
	Schema      string       `json:"schema,omitempty"`
	Description string       `json:"description,omitempty"`
	Fields      []ModelField `json:"fields,omitempty"`
	Meta        []ModelMeta  `json:"meta,omitempty"`
}

type ModelField struct {
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type ModelMeta struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ModelEdge struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	Label       string `json:"label,omitempty"`
	Kind        string `json:"kind"`
	SourceField string `json:"sourceField,omitempty"`
	TargetField string `json:"targetField,omitempty"`
	Cardinality string `json:"cardinality,omitempty"`
}

type Filters struct {
	DateRange        string            `json:"dateRange"`
	State            string            `json:"state"`
	Category         string            `json:"category"`
	VisualSelections []VisualSelection `json:"visualSelections"`
}

type Runtime struct {
	ClientID string `json:"clientId"`
}

func (f Filters) WithDefaults() Filters {
	if f.DateRange == "" {
		f.DateRange = "all"
	}
	if f.State == "" {
		f.State = "all"
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

type ChartCommand struct {
	VisualID string `json:"visualId"`
	Field    string `json:"field"`
	Value    string `json:"value"`
	Label    string `json:"label"`
	Mode     string `json:"mode"`
}

func (c ChartCommand) IsEmpty() bool {
	return c.VisualID == "" || c.Field == "" || c.Value == ""
}

func (f Filters) ToggleSelection(command ChartCommand) Filters {
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
	Filters Filters          `json:"filters"`
	Status  Status           `json:"status"`
	KPIs    []KPI            `json:"kpis"`
	Charts  map[string]Chart `json:"charts"`
}

type Status struct {
	Loading       bool   `json:"loading"`
	Error         string `json:"error"`
	LastUpdated   string `json:"lastUpdated"`
	DataDirectory string `json:"dataDirectory"`
	SetupRequired bool   `json:"setupRequired"`
}

type KPI struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
	Tone  string `json:"tone"`
}

type Chart struct {
	Version   int      `json:"version"`
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	Unit      string   `json:"unit"`
	Field     string   `json:"field"`
	Selection []string `json:"selection"`
	Data      []Point  `json:"data"`
}

type Point struct {
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Selected bool    `json:"selected,omitempty"`
}

type TableRequest struct {
	Table  string    `json:"table"`
	Offset int       `json:"offset"`
	Limit  int       `json:"limit"`
	Sort   TableSort `json:"sort"`
}

func DefaultTableRequest() TableRequest {
	return TableRequest{
		Table:  "orders",
		Offset: 0,
		Limit:  120,
		Sort:   TableSort{Key: "purchase_date", Direction: "desc"},
	}
}

func (r TableRequest) WithDefaults() TableRequest {
	defaults := DefaultTableRequest()
	if r.Table == "" {
		r.Table = defaults.Table
	}
	if r.Limit <= 0 {
		r.Limit = defaults.Limit
	}
	if r.Limit > 500 {
		r.Limit = 500
	}
	if r.Offset < 0 {
		r.Offset = 0
	}
	if r.Sort.Key == "" {
		r.Sort = defaults.Sort
	}
	if r.Sort.Direction != "asc" && r.Sort.Direction != "desc" {
		r.Sort.Direction = defaults.Sort.Direction
	}
	return r
}

type TableSort struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
}

type Table struct {
	Title     string           `json:"title"`
	Columns   []TableColumn    `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	TotalRows int              `json:"totalRows"`
	Window    TableWindow      `json:"window"`
	Sort      TableSort        `json:"sort"`
	Loading   bool             `json:"loading"`
	Error     string           `json:"error"`
}

type TableWindow struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type TableColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Align string `json:"align,omitempty"`
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
		Title:     "Orders",
		Columns:   OrdersTableColumns(),
		Rows:      []map[string]any{},
		TotalRows: 0,
		Window:    TableWindow{Offset: request.Offset, Limit: request.Limit},
		Sort:      request.Sort,
		Loading:   false,
		Error:     message,
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
		KPIs: []KPI{
			{Label: "Orders", Value: "-", Note: "Waiting for CSVs", Tone: "neutral"},
			{Label: "Revenue", Value: "-", Note: "Waiting for CSVs", Tone: "neutral"},
			{Label: "AOV", Value: "-", Note: "Waiting for CSVs", Tone: "neutral"},
			{Label: "Review", Value: "-", Note: "Waiting for CSVs", Tone: "neutral"},
		},
		Charts: map[string]Chart{
			"revenue":    {Version: 1, ID: "revenue", Type: "area", Title: "Revenue by month", Unit: "R$", Field: "purchase_month", Selection: []string{}, Data: []Point{}},
			"orders":     {Version: 1, ID: "orders", Type: "donut", Title: "Orders by status", Unit: "orders", Field: "status", Selection: []string{}, Data: []Point{}},
			"categories": {Version: 1, ID: "categories", Type: "bar", Title: "Top product categories", Unit: "R$", Field: "category", Selection: []string{}, Data: []Point{}},
			"delivery":   {Version: 1, ID: "delivery", Type: "bar", Title: "Delivery speed", Unit: "orders", Field: "delivery_bucket", Selection: []string{}, Data: []Point{}},
		},
	}
}
