package query

type Field struct {
	Field string
	Alias string
}

type Time struct {
	Field string
	Grain string
	Alias string
}

type Filter struct {
	Field    string
	Operator string
	Values   []any
}

type Sort struct {
	Field     string
	Direction string
}

type Request struct {
	MetricView string
	Dimensions []Field
	Measures   []Field
	Time       Time
	Filters    []Filter
	Sort       []Sort
	Limit      int
}

type RowRequest struct {
	MetricView string
	Dimensions []Field
	Measures   []Field
	Filters    []Filter
	Sort       []Sort
	Limit      int
	Offset     int
}

type RawValueRequest struct {
	MetricView string
	Dimensions []Field
	Measure    Field
	Filters    []Filter
	Sort       []Sort
	Limit      int
}

type CountRequest struct {
	MetricView string
	Filters    []Filter
}

type Plan struct {
	SQL     string
	Args    []any
	Columns []string
}
