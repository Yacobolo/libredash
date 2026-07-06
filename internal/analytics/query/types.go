package query

type Field struct {
	Field   string
	Alias   string
	Measure InlineMeasure
}

type InlineMeasure struct {
	Field       string
	Name        string
	Label       string
	Description string
	Expr        string
	Expression  string
	Table       string
	Grain       string
	Time        string
	Grains      []string
	Unit        string
	Format      string
}

func (m InlineMeasure) SQLExpression() string {
	if m.Expression != "" {
		return m.Expression
	}
	return m.Expr
}

type ResolvedMeasure struct {
	Field       string
	Name        string
	Label       string
	Description string
	Expr        string
	Expression  string
	Table       string
	Grain       string
	Time        string
	Grains      []string
	Unit        string
	Format      string
}

func (m ResolvedMeasure) SQLExpression() string {
	if m.Expression != "" {
		return m.Expression
	}
	return m.Expr
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
	Groups   []FilterGroup
}

type FilterGroup struct {
	Filters []Filter
}

type Sort struct {
	Field     string
	Direction string
}

type ColumnMask struct {
	Field string
	Mask  string
}

type Request struct {
	Table       string
	Dimensions  []Field
	Measures    []Field
	Time        Time
	Filters     []Filter
	Sort        []Sort
	ColumnMasks []ColumnMask
	Limit       int
	Offset      int
}

type RowRequest struct {
	Table       string
	Dimensions  []Field
	Measures    []Field
	Filters     []Filter
	Sort        []Sort
	ColumnMasks []ColumnMask
	Limit       int
	Offset      int
}

type RawValueRequest struct {
	Table       string
	Dimensions  []Field
	Measure     Field
	Filters     []Filter
	Sort        []Sort
	ColumnMasks []ColumnMask
	Limit       int
}

type CountRequest struct {
	Table   string
	Filters []Filter
}

type Plan struct {
	SQL     string
	Args    []any
	Columns []string
}
