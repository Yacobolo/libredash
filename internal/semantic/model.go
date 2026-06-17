package semantic

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var semanticIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Model struct {
	Name              string                `yaml:"name"`
	Title             string                `yaml:"title"`
	Description       string                `yaml:"description"`
	DefaultConnection string                `yaml:"default_connection"`
	Connections       map[string]Connection `yaml:"connections"`
	Sources           map[string]Source     `yaml:"sources"`
	Cache             Cache                 `yaml:"cache"`
	Datasets          map[string]Dataset    `yaml:"datasets"`
	Relationships     []Relationship        `yaml:"relationships"`
}

type Connection struct {
	Kind     string             `yaml:"kind"`
	Root     string             `yaml:"root"`
	Secret   string             `yaml:"secret"`
	Scope    string             `yaml:"scope"`
	Auth     ConnectionAuth     `yaml:"auth"`
	Options  map[string]any     `yaml:"options"`
	Defaults ConnectionDefaults `yaml:"defaults"`
}

type ConnectionDefaults struct {
	Options map[string]any `yaml:"options"`
}

type ConnectionAuth struct {
	Method  string         `yaml:"method"`
	Profile string         `yaml:"profile"`
	Chain   string         `yaml:"chain"`
	Account string         `yaml:"account"`
	Params  map[string]any `yaml:"params"`
}

type Source struct {
	Format     string         `yaml:"format"`
	Location   string         `yaml:"location"`
	Connection string         `yaml:"connection"`
	Object     string         `yaml:"object"`
	Options    map[string]any `yaml:"options"`
}

type Cache struct {
	Tables map[string]CacheTable `yaml:"tables"`
}

type CacheTable struct {
	Description string `yaml:"description"`
	SQL         string `yaml:"sql"`
}

type Dataset struct {
	Source string `yaml:"source"`
}

type MetricView struct {
	ID            string                     `yaml:"id"`
	Title         string                     `yaml:"title"`
	Description   string                     `yaml:"description"`
	SemanticModel string                     `yaml:"semantic_model"`
	Dataset       string                     `yaml:"dataset"`
	Timeseries    string                     `yaml:"timeseries"`
	Dimensions    map[string]MetricDimension `yaml:"dimensions"`
	Measures      map[string]MetricMeasure   `yaml:"measures"`
}

type MetricDimension struct {
	Label     string `yaml:"label"`
	Expr      string `yaml:"expr"`
	Where     string `yaml:"where"`
	OrderExpr string `yaml:"order_expr"`
}

type MetricMeasure struct {
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

func Load(path string) (*Model, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var model Model
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&model); err != nil {
		return nil, err
	}
	if err := model.Validate(); err != nil {
		return nil, err
	}
	return &model, nil
}

func (m *Model) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("semantic model name is required")
	}
	if len(m.Sources) == 0 {
		return fmt.Errorf("semantic model %q has no sources", m.Name)
	}
	if len(m.Cache.Tables) == 0 {
		return fmt.Errorf("semantic model %q has no cache tables", m.Name)
	}
	for name, connection := range m.Connections {
		if err := connection.Validate(name); err != nil {
			return err
		}
	}
	if m.DefaultConnection != "" {
		if err := validateSemanticIdentifier(m.DefaultConnection); err != nil {
			return fmt.Errorf("default_connection %q is invalid: %w", m.DefaultConnection, err)
		}
		if _, ok := m.Connections[m.DefaultConnection]; !ok {
			return fmt.Errorf("default_connection %q references unknown connection", m.DefaultConnection)
		}
	}
	for name, source := range m.Sources {
		resolved, err := m.resolveSource(source)
		if err != nil {
			return fmt.Errorf("source %q: %w", name, err)
		}
		if err := resolved.Validate(name, m.Connections); err != nil {
			return err
		}
		m.Sources[name] = resolved
	}
	for name, table := range m.Cache.Tables {
		if table.SQL == "" {
			return fmt.Errorf("cache table %q is missing sql", name)
		}
	}
	if len(m.Datasets) == 0 {
		return fmt.Errorf("semantic model %q has no datasets", m.Name)
	}
	for name, dataset := range m.Datasets {
		if dataset.Source == "" {
			return fmt.Errorf("dataset %q requires source", name)
		}
		if _, ok := m.Cache.Tables[dataset.Source]; !ok {
			return fmt.Errorf("dataset %q references unknown cache table %q", name, dataset.Source)
		}
	}
	seenRelationships := map[string]struct{}{}
	for index, relationship := range m.Relationships {
		if relationship.ID == "" || relationship.From == "" || relationship.To == "" {
			return fmt.Errorf("relationship %d requires id, from, and to", index)
		}
		if _, exists := seenRelationships[relationship.ID]; exists {
			return fmt.Errorf("duplicate relationship id %q", relationship.ID)
		}
		seenRelationships[relationship.ID] = struct{}{}
	}
	return nil
}

func (m *Model) resolveSource(source Source) (Source, error) {
	switch source.Kind() {
	case "location", "database":
		if source.Connection == "" {
			source.Connection = m.DefaultConnection
		}
		if source.Connection == "" {
			return source, fmt.Errorf("requires connection")
		}
		connection, ok := m.Connections[source.Connection]
		if !ok {
			return source, fmt.Errorf("references unknown connection %q", source.Connection)
		}
		if source.Location != "" {
			if len(connection.Defaults.Options) > 0 {
				options := make(map[string]any, len(connection.Defaults.Options)+len(source.Options))
				for key, value := range connection.Defaults.Options {
					options[key] = value
				}
				for key, value := range source.Options {
					options[key] = value
				}
				source.Options = options
			}
			if source.Format == "" {
				format, ok := inferSourceFormat(source.Location)
				if !ok {
					return source, fmt.Errorf("location %q requires format", source.Location)
				}
				source.Format = format
			}
		}
		return source, nil
	default:
		return source, nil
	}
}

func LoadMetricView(path string, model *Model) (*MetricView, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var view MetricView
	if err := yaml.Unmarshal(bytes, &view); err != nil {
		return nil, err
	}
	if err := view.Validate(model); err != nil {
		return nil, err
	}
	return &view, nil
}

func (v *MetricView) Validate(model *Model) error {
	if v.ID == "" || v.Title == "" || v.SemanticModel == "" || v.Dataset == "" {
		return fmt.Errorf("metrics view requires id, title, semantic_model, and dataset")
	}
	if model == nil {
		return fmt.Errorf("metrics view %q requires semantic model %q", v.ID, v.SemanticModel)
	}
	if v.SemanticModel != model.Name {
		return fmt.Errorf("metrics view %q semantic_model %q does not match model %q", v.ID, v.SemanticModel, model.Name)
	}
	if _, ok := model.Datasets[v.Dataset]; !ok {
		return fmt.Errorf("metrics view %q references unknown dataset %q", v.ID, v.Dataset)
	}
	if v.Timeseries == "" {
		return fmt.Errorf("metrics view %q requires timeseries", v.ID)
	}
	if len(v.Dimensions) == 0 {
		return fmt.Errorf("metrics view %q requires dimensions", v.ID)
	}
	if len(v.Measures) == 0 {
		return fmt.Errorf("metrics view %q requires measures", v.ID)
	}
	if _, ok := v.Dimensions[v.Timeseries]; !ok {
		return fmt.Errorf("metrics view %q timeseries %q is not a dimension", v.ID, v.Timeseries)
	}
	for name, dimension := range v.Dimensions {
		if dimension.Expr == "" {
			return fmt.Errorf("metrics view %q dimension %q requires expr", v.ID, name)
		}
	}
	for name, measure := range v.Measures {
		if measure.Label == "" || measure.Expression == "" {
			return fmt.Errorf("metrics view %q measure %q requires label and expression", v.ID, name)
		}
	}
	return nil
}

func (s Source) Validate(name string, connections map[string]Connection) error {
	if err := validateSemanticIdentifier(name); err != nil {
		return fmt.Errorf("source %q has invalid name: %w", name, err)
	}
	for key := range s.Options {
		if err := validateSemanticIdentifier(key); err != nil {
			return fmt.Errorf("source %q option %q is invalid: %w", name, key, err)
		}
	}
	switch s.Kind() {
	case "location":
		if s.Connection == "" {
			return fmt.Errorf("source %q requires connection", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		if !supportsLocationConnection(connection.Kind) {
			return fmt.Errorf("source %q location cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
		if connection.Kind == "local" && !isLocalLocation(s.Location) {
			return fmt.Errorf("source %q local connection %q cannot use remote location %q", name, s.Connection, s.Location)
		}
		if isRemoteConnection(connection.Kind) && isLocalLocation(s.Location) && connection.Scope == "" {
			return fmt.Errorf("source %q remote connection %q requires scope for relative location %q", name, s.Connection, s.Location)
		}
		if s.Format == "" {
			return fmt.Errorf("source %q location requires format", name)
		}
		if !supportsSourceFormat(s.Format) {
			return fmt.Errorf("source %q has unsupported format %q", name, s.Format)
		}
	case "database":
		if s.Connection == "" {
			return fmt.Errorf("source %q database object requires connection", name)
		}
		if s.Format != "" || len(s.Options) > 0 {
			return fmt.Errorf("source %q database object cannot set format or options", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		if !supportsDatabaseConnection(connection.Kind) {
			return fmt.Errorf("source %q object cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
	default:
		return fmt.Errorf("source %q requires exactly one of location or object", name)
	}
	return nil
}

func (c Connection) Validate(name string) error {
	if err := validateSemanticIdentifier(name); err != nil {
		return fmt.Errorf("connection %q has invalid name: %w", name, err)
	}
	if c.Kind == "" {
		return fmt.Errorf("connection %q requires kind", name)
	}
	if !supportsConnectionKind(c.Kind) {
		return fmt.Errorf("connection %q has unsupported kind %q", name, c.Kind)
	}
	if c.Secret != "" {
		if err := validateSemanticIdentifier(c.Secret); err != nil {
			return fmt.Errorf("connection %q secret %q is invalid: %w", name, c.Secret, err)
		}
	}
	if c.Auth.Method != "" && !supportsAuthMethod(c.Auth.Method) {
		return fmt.Errorf("connection %q has unsupported auth method %q", name, c.Auth.Method)
	}
	for key := range c.Auth.Params {
		if err := validateSemanticIdentifier(key); err != nil {
			return fmt.Errorf("connection %q auth param %q is invalid: %w", name, key, err)
		}
	}
	for key := range c.Options {
		if !supportsConnectionOption(key) {
			return fmt.Errorf("connection %q has unsupported option %q", name, key)
		}
	}
	for key := range c.Defaults.Options {
		if err := validateSemanticIdentifier(key); err != nil {
			return fmt.Errorf("connection %q default option %q is invalid: %w", name, key, err)
		}
	}
	return nil
}

func (s Source) Description() string {
	switch s.Kind() {
	case "location":
		if supportsLakehouseFormat(s.Format) {
			return s.Format + " table: " + s.Location
		}
		return s.Format + " file: " + s.Location
	case "database":
		return "database object: " + s.Object
	default:
		return "source"
	}
}

func (s Source) Role() string {
	switch s.Kind() {
	case "location":
		return s.Format
	case "database":
		return "database"
	default:
		return "source"
	}
}

func (s Source) Kind() string {
	count := 0
	kind := ""
	if s.Location != "" {
		count++
		kind = "location"
	}
	if s.Object != "" {
		count++
		kind = "database"
	}
	if count != 1 {
		return ""
	}
	return kind
}

func isLocalLocation(location string) bool {
	for _, prefix := range []string{"s3://", "r2://", "gcs://", "gs://", "az://", "azure://", "abfss://", "http://", "https://", "file://"} {
		if strings.HasPrefix(location, prefix) {
			return false
		}
	}
	return !strings.Contains(location, "://")
}

func supportsConnectionKind(kind string) bool {
	switch kind {
	case "local", "s3", "r2", "gcs", "http", "azure_blob", "postgres", "mysql", "sqlite":
		return true
	default:
		return false
	}
}

func supportsAuthMethod(method string) bool {
	switch method {
	case "credential_chain", "config":
		return true
	default:
		return false
	}
}

func supportsConnectionOption(option string) bool {
	switch option {
	case "connection_string", "uri", "path", "database":
		return true
	default:
		return false
	}
}

func validateSemanticIdentifier(value string) error {
	if !semanticIdentifierPattern.MatchString(value) {
		return fmt.Errorf("must match %s", semanticIdentifierPattern.String())
	}
	return nil
}

func supportsFileFormat(format string) bool {
	switch format {
	case "csv", "json", "parquet", "excel", "text", "blob", "vortex":
		return true
	default:
		return false
	}
}

func supportsLakehouseFormat(format string) bool {
	switch format {
	case "delta", "iceberg":
		return true
	default:
		return false
	}
}

func supportsSourceFormat(format string) bool {
	return supportsFileFormat(format) || supportsLakehouseFormat(format)
}

func supportsLocationConnection(kind string) bool {
	switch kind {
	case "local", "s3", "r2", "gcs", "http", "azure_blob":
		return true
	default:
		return false
	}
}

func supportsDatabaseConnection(kind string) bool {
	switch kind {
	case "postgres", "mysql", "sqlite":
		return true
	default:
		return false
	}
}

func isRemoteConnection(kind string) bool {
	return supportsLocationConnection(kind) && kind != "local"
}

func inferSourceFormat(location string) (string, bool) {
	base, compression := splitCompressionSuffix(location)
	ext := strings.ToLower(filepathExt(base))
	switch {
	case ext == ".csv" && (compression == "" || compression == ".gz"):
		return "csv", true
	case ext == ".json", ext == ".jsonl", ext == ".ndjson":
		return "json", true
	case ext == ".parquet":
		return "parquet", true
	case ext == ".xlsx":
		return "excel", true
	case ext == ".txt":
		return "text", true
	case ext == ".blob":
		return "blob", true
	case ext == ".vortex":
		return "vortex", true
	default:
		return "", false
	}
}

func splitCompressionSuffix(location string) (string, string) {
	if strings.HasSuffix(strings.ToLower(location), ".gz") {
		return location[:len(location)-3], ".gz"
	}
	return location, ""
}

func filepathExt(location string) string {
	if index := strings.LastIndexAny(location, `/\`); index >= 0 {
		location = location[index+1:]
	}
	if index := strings.LastIndex(location, "."); index >= 0 {
		return location[index:]
	}
	return ""
}

func (m *Model) CacheTableNames() []string {
	names := make([]string, 0, len(m.Cache.Tables))
	for name := range m.Cache.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func supportsVisualKind(kind string) bool {
	return kind == "chart"
}

func supportsVisualShape(shape string) bool {
	switch shape {
	case "category_value", "category_series_value", "category_multi_measure", "category_delta", "single_value", "matrix", "graph", "geo", "ohlc", "distribution", "binned_measure", "hierarchy":
		return true
	default:
		return false
	}
}

func supportsRenderer(renderer string) bool {
	return renderer == "echarts"
}

func rendererSupportsType(renderer, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst":
		return true
	default:
		return false
	}
}

func supportsSeries(shape string) bool {
	return shape == "category_series_value"
}

func rendererSupportsShapeType(renderer, shape, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch shape {
	case "category_value":
		switch chartType {
		case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "radar":
			return true
		}
	case "category_series_value":
		return rendererTypeSupportsSeries(renderer, chartType)
	case "category_multi_measure":
		return chartType == "combo"
	case "category_delta":
		return chartType == "waterfall"
	case "single_value":
		return chartType == "gauge"
	case "matrix":
		return chartType == "heatmap"
	case "graph":
		return chartType == "sankey" || chartType == "graph"
	case "geo":
		return chartType == "map"
	case "ohlc":
		return chartType == "candlestick"
	case "distribution":
		return chartType == "boxplot"
	case "binned_measure":
		return chartType == "histogram"
	case "hierarchy":
		return chartType == "tree" || chartType == "sunburst"
	}
	return false
}

func rendererTypeSupportsSeries(renderer, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "scatter":
		return true
	default:
		return false
	}
}
