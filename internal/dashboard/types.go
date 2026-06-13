package dashboard

type Signals struct {
	Filters      Filters      `json:"filters"`
	Runtime      Runtime      `json:"runtime"`
	TableCommand TableRequest `json:"tableCommand"`
}

type Filters struct {
	DateRange string `json:"dateRange"`
	State     string `json:"state"`
	Category  string `json:"category"`
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
	Title string  `json:"title"`
	Unit  string  `json:"unit"`
	Data  []Point `json:"data"`
}

type Point struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
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
			"revenue":    {Title: "Revenue by month", Unit: "R$", Data: []Point{}},
			"orders":     {Title: "Orders by status", Unit: "orders", Data: []Point{}},
			"categories": {Title: "Top product categories", Unit: "R$", Data: []Point{}},
			"delivery":   {Title: "Delivery delay", Unit: "orders", Data: []Point{}},
		},
	}
}
