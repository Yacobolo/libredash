package model

import "regexp"

var (
	semanticIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type Model struct {
	Name              string                       `yaml:"-"`
	Title             string                       `yaml:"-"`
	Description       string                       `yaml:"-"`
	DefaultConnection string                       `yaml:"-"`
	Connections       map[string]Connection        `yaml:"-"`
	Sources           map[string]Source            `yaml:"-"`
	Tables            map[string]Table             `yaml:"-"`
	Relationships     []Relationship               `yaml:"-"`
	Measures          map[string]MetricMeasure     `yaml:"-"`
	Dimensions        map[string]SemanticDimension `yaml:"-"`
	Metrics           map[string]Metric            `yaml:"-"`
}

type Connection struct {
	Kind        string `yaml:"kind"`
	Description string `yaml:"description"`
	Path        string `yaml:"path"`
	Root        string `yaml:"root"`
	Scope       string `yaml:"scope"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	Database    string `yaml:"database"`
	Username    string `yaml:"username"`
	SSLMode     string `yaml:"sslMode"`
	// Auth is populated only on a short-lived refresh copy by the injected
	// credential resolver. It is deliberately absent from authored contracts.
	Auth        ConnectionAuth        `yaml:"-" json:"-"`
	Credentials ConnectionCredentials `yaml:"credentials" json:"credentials,omitempty"`
	Options     map[string]any        `yaml:"options"`
	Defaults    ConnectionDefaults    `yaml:"defaults"`
}

type ConnectionCredentials struct {
	Provider    string `yaml:"provider" json:"provider"`
	Secret      string `yaml:"secret" json:"secret,omitempty"`
	Region      string `yaml:"region" json:"region,omitempty"`
	Endpoint    string `yaml:"endpoint" json:"endpoint,omitempty"`
	AccountName string `yaml:"accountName" json:"accountName,omitempty"`
}

type ConnectionDefaults struct {
	Options map[string]any `yaml:"options"`
}

type ConnectionAuth map[string]any

type Source struct {
	Format      string                 `yaml:"format"`
	Description string                 `yaml:"description"`
	Path        string                 `yaml:"path"`
	Connection  string                 `yaml:"connection"`
	Object      string                 `yaml:"object"`
	Options     map[string]any         `yaml:"options"`
	Fields      map[string]SourceField `yaml:"fields"`
	Schema      TableSchema            `yaml:"-"`
}

type Table struct {
	Source             string                     `yaml:"source"`
	Sources            []string                   `yaml:"sources"`
	SourceReads        map[string][]string        `yaml:"source_reads"`
	SQL                string                     `yaml:"sql"`
	Transform          Transform                  `yaml:"transform"`
	Columns            map[string]ModelColumn     `yaml:"columns"`
	PrimaryKey         string                     `yaml:"primary_key"`
	Grain              string                     `yaml:"grain"`
	Dimensions         map[string]MetricDimension `yaml:"fields"`
	Description        string                     `yaml:"description"`
	Schema             TableSchema                `yaml:"-"`
	SourceDependencies []string                   `yaml:"-"`
	ModelDependencies  []string                   `yaml:"-"`
}

type Transform struct {
	SQL string `yaml:"sql"`
}

type SourceField struct {
	Field       string `yaml:"-"`
	Table       string `yaml:"-"`
	Name        string `yaml:"-"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
}

type ModelColumn struct {
	Field       string `yaml:"-"`
	Name        string `yaml:"-"`
	SourceField string `yaml:"source_field"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
}

type MetricDimension struct {
	Field       string `yaml:"-"`
	Table       string `yaml:"-"`
	Name        string `yaml:"-"`
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Type        string `yaml:"-" json:"-"`
	Expr        string `yaml:"-" json:"-"`
	Expression  string `yaml:"-" json:"-"`
}

type TableSchema struct {
	Columns []ColumnSchema `json:"columns,omitempty"`
}

type ColumnSchema struct {
	Name         string `json:"name"`
	Ordinal      int    `json:"ordinal"`
	PhysicalType string `json:"physicalType"`
	Nullable     *bool  `json:"nullable,omitempty"`
	Default      string `json:"default,omitempty"`
	Comment      string `json:"comment,omitempty"`
	PrimaryKey   bool   `json:"primaryKey,omitempty"`
}

type MetricMeasure struct {
	Field       string          `yaml:"-"`
	Name        string          `yaml:"-"`
	Fact        string          `yaml:"fact"`
	Label       string          `yaml:"label"`
	Description string          `yaml:"description"`
	Aggregation string          `yaml:"aggregation"`
	Input       MeasureInput    `yaml:"input"`
	Filters     []MeasureFilter `yaml:"filters"`
	Empty       string          `yaml:"empty"`
	Unit        string          `yaml:"unit"`
	Format      string          `yaml:"format"`
	Hidden      bool            `yaml:"hidden"`
}

type MeasureInput struct {
	Field      string `yaml:"field"`
	Expression string `yaml:"expression"`
}

type MeasureFilter struct {
	Field    string `yaml:"field"`
	Operator string `yaml:"operator"`
	Values   []any  `yaml:"values"`
}

type SemanticDimension struct {
	Name        string                      `yaml:"-"`
	Label       string                      `yaml:"label"`
	Description string                      `yaml:"description"`
	Type        string                      `yaml:"type"`
	Grains      []string                    `yaml:"grains"`
	Bindings    map[string]DimensionBinding `yaml:"bindings"`
}

type DimensionBinding struct {
	Field string   `yaml:"field"`
	Path  []string `yaml:"path"`
}

type Metric struct {
	Name        string `yaml:"-"`
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	Expression  string `yaml:"expression"`
	Unit        string `yaml:"unit"`
	Format      string `yaml:"format"`
	Hidden      bool   `yaml:"hidden"`
}

type Relationship struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Cardinality string `yaml:"cardinality"`
}
