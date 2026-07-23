package query

import semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"

type Field struct {
	Field string
	Alias string
}

type ResolvedMeasure struct {
	Field           string
	Name            string
	Label           string
	Description     string
	Fact            string
	Aggregation     string
	InputField      string
	InputExpr       string
	InputExpression *semanticmodel.Expression
	Filters         []MeasureFilter
	Empty           string
	Unit            string
	Format          string
}

type MeasureFilter struct {
	Field    string
	Operator string
	Values   []any
}

type Time struct {
	Field string
	Grain string
	Alias string
}

type Filter struct {
	Field    string
	Fact     string
	Operator string
	Values   []any
	Groups   []FilterGroup
	Spatial  *SpatialFilter
}

type SpatialFilter struct {
	Kind           string
	LatitudeField  string
	LongitudeField string
	Fact           string
	West           float64
	South          float64
	East           float64
	North          float64
	Points         []SpatialPoint
	Center         SpatialPoint
	RadiusMeters   float64
}

type SpatialPoint struct {
	Longitude float64
	Latitude  float64
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

type SpatialPrecision string

const (
	SpatialPrecisionRaw        SpatialPrecision = "raw"
	SpatialPrecisionAggregated SpatialPrecision = "aggregated"
)

// SpatialRequest projects a governed semantic rowset into one bounded map
// viewport. Aggregated precision groups the complete filtered rowset into a
// deterministic screen-space grid before the feature cap is applied.
type SpatialRequest struct {
	Table       string
	Dimensions  []Field
	Measures    []Field
	Time        Time
	Filters     []Filter
	Sort        []Sort
	ColumnMasks []ColumnMask
	Latitude    Field
	Longitude   Field
	West        float64
	South       float64
	East        float64
	North       float64
	Width       int
	Height      int
	FeatureCap  int
	Precision   SpatialPrecision
}

type Plan struct {
	SQL                  string
	Args                 []any
	Columns              []string
	Mode                 string
	Facts                []string
	StitchDimensions     []string
	PhysicalDependencies []string
	RelationshipPaths    []string
}

// BundleRequest is one independently shaped aggregate in a shared governed
// single-fact scan. ID is an opaque consumer key and must be unique in a
// bundle.
type BundleRequest struct {
	ID      string
	Request Request
}

// BundlePlan is one physical statement containing independently shaped result
// branches over a common governed scan.
type BundlePlan struct {
	Plan     Plan
	Branches []BundleBranch
}

type BundleBranch struct {
	ID      string
	Ordinal int
	Columns []BundleColumn
}

type BundleColumn struct {
	Output   string
	Physical string
}

const (
	BundleBranchColumn = "__bundle_branch"
	BundleRowColumn    = "__bundle_row"
)
