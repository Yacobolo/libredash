package semantic

import "regexp"

var (
	semanticIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	envReferencePattern       = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
)

type Model struct {
	Name              string                `yaml:"name"`
	Title             string                `yaml:"title"`
	Description       string                `yaml:"description"`
	DefaultConnection string                `yaml:"default_connection"`
	Connections       map[string]Connection `yaml:"connections"`
	Sources           map[string]Source     `yaml:"sources"`
	Tables            map[string]ModelTable `yaml:"tables"`
	Relationships     []Relationship        `yaml:"relationships"`
}

type Connection struct {
	Kind     string             `yaml:"kind"`
	Path     string             `yaml:"path"`
	Root     string             `yaml:"root"`
	Scope    string             `yaml:"scope"`
	Auth     ConnectionAuth     `yaml:"auth"`
	Options  map[string]any     `yaml:"options"`
	Defaults ConnectionDefaults `yaml:"defaults"`
}

type ConnectionDefaults struct {
	Options map[string]any `yaml:"options"`
}

type ConnectionAuth map[string]any

type Source struct {
	Format     string         `yaml:"format"`
	Path       string         `yaml:"path"`
	Connection string         `yaml:"connection"`
	Object     string         `yaml:"object"`
	Options    map[string]any `yaml:"options"`
}

type ModelTable struct {
	Kind        string                     `yaml:"kind"`
	Source      string                     `yaml:"source"`
	Transform   ModelTransform             `yaml:"transform"`
	PrimaryKey  string                     `yaml:"primary_key"`
	Grain       string                     `yaml:"grain"`
	Dimensions  map[string]MetricDimension `yaml:"dimensions"`
	Measures    map[string]MetricMeasure   `yaml:"measures"`
	Description string                     `yaml:"description"`
}

type ModelTransform struct {
	SQL string `yaml:"sql"`
}

type MetricView struct {
	ID            string                     `yaml:"id"`
	Title         string                     `yaml:"title"`
	Description   string                     `yaml:"description"`
	SemanticModel string                     `yaml:"semantic_model"`
	BaseTable     string                     `yaml:"base_table"`
	Grain         string                     `yaml:"grain"`
	Time          ViewTime                   `yaml:"time"`
	DimensionRefs []string                   `yaml:"dimensions"`
	MeasureRefs   []string                   `yaml:"measures"`
	Dimensions    map[string]MetricDimension `yaml:"-"`
	Measures      map[string]MetricMeasure   `yaml:"-"`
}

type ViewTime struct {
	DefaultField  string   `yaml:"default_field"`
	AllowedGrains []string `yaml:"allowed_grains"`
}

type MetricDimension struct {
	Field      string `yaml:"-"`
	Table      string `yaml:"-"`
	Name       string `yaml:"-"`
	Label      string `yaml:"label"`
	Expr       string `yaml:"expr"`
	Expression string `yaml:"expression"`
	Where      string `yaml:"where"`
	OrderExpr  string `yaml:"order_expr"`
	Type       string `yaml:"type"`
}

type MetricMeasure struct {
	Field       string `yaml:"-"`
	Table       string `yaml:"-"`
	Name        string `yaml:"-"`
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Expression  string `yaml:"expression"`
	Unit        string `yaml:"unit"`
	Format      string `yaml:"format"`
}

type Relationship struct {
	ID          string `yaml:"id"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
	Active      bool   `yaml:"active"`
}
