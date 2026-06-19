package semantic

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	sourcereg "github.com/Yacobolo/libredash/internal/source"
	"gopkg.in/yaml.v3"
)

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
	for name, connection := range m.Connections {
		resolved, err := connection.Validate(name)
		if err != nil {
			return err
		}
		m.Connections[name] = resolved
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
	if len(m.Tables) == 0 {
		return fmt.Errorf("semantic model %q has no model tables", m.Name)
	}
	for name, table := range m.Tables {
		if err := validateSemanticIdentifier(name); err != nil {
			return fmt.Errorf("model table %q has invalid name: %w", name, err)
		}
		if table.Kind == "" {
			return fmt.Errorf("model table %q requires kind", name)
		}
		if (table.Source == "") == (table.Transform.SQL == "") {
			return fmt.Errorf("model table %q requires exactly one of source or transform.sql", name)
		}
		if table.Source != "" {
			if _, ok := m.Sources[table.Source]; !ok {
				return fmt.Errorf("model table %q references unknown source %q", name, table.Source)
			}
		}
		if table.PrimaryKey == "" {
			return fmt.Errorf("model table %q requires primary_key", name)
		}
		if table.Grain == "" {
			return fmt.Errorf("model table %q requires grain", name)
		}
		for field, dimension := range table.Dimensions {
			if err := validateSemanticIdentifier(field); err != nil {
				return fmt.Errorf("model table %q dimension %q is invalid: %w", name, field, err)
			}
			if dimension.SQLExpression() == "" {
				return fmt.Errorf("model table %q dimension %q requires expr", name, field)
			}
		}
		for field, measure := range table.Measures {
			if err := validateSemanticIdentifier(field); err != nil {
				return fmt.Errorf("model table %q measure %q is invalid: %w", name, field, err)
			}
			if measure.Label == "" || strings.TrimSpace(measure.Expression) == "" {
				return fmt.Errorf("model table %q measure %q requires label and expression", name, field)
			}
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
	case sourcereg.KindPath, sourcereg.KindObject:
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
		if source.Path != "" {
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
				format, ok := sourcereg.InferFormat(source.Path)
				if !ok {
					return source, fmt.Errorf("path %q requires format", source.Path)
				}
				source.Format = format
			}
		}
		return source, nil
	default:
		return source, nil
	}
}

func (v *MetricView) Validate(model *Model) error {
	if v.ID == "" || v.Title == "" || v.SemanticModel == "" || v.BaseTable == "" {
		return fmt.Errorf("metric view requires id, title, semantic_model, and base_table")
	}
	if model == nil {
		return fmt.Errorf("metric view %q requires semantic model %q", v.ID, v.SemanticModel)
	}
	if v.SemanticModel != model.Name {
		return fmt.Errorf("metric view %q semantic_model %q does not match model %q", v.ID, v.SemanticModel, model.Name)
	}
	base, ok := model.Tables[v.BaseTable]
	if !ok {
		return fmt.Errorf("metric view %q references unknown base table %q", v.ID, v.BaseTable)
	}
	if v.Grain == "" {
		v.Grain = base.Grain
	}
	if v.Grain == "" {
		return fmt.Errorf("metric view %q requires grain", v.ID)
	}
	if v.Time.DefaultField == "" {
		return fmt.Errorf("metric view %q requires time.default_field", v.ID)
	}
	if len(v.DimensionRefs) == 0 {
		return fmt.Errorf("metric view %q requires dimensions", v.ID)
	}
	if len(v.MeasureRefs) == 0 {
		return fmt.Errorf("metric view %q requires measures", v.ID)
	}
	v.Dimensions = map[string]MetricDimension{}
	for _, ref := range v.DimensionRefs {
		dimension, err := model.ResolveDimension(ref)
		if err != nil {
			return fmt.Errorf("metric view %q dimension %q: %w", v.ID, ref, err)
		}
		v.Dimensions[ref] = dimension
	}
	v.Measures = map[string]MetricMeasure{}
	for _, ref := range v.MeasureRefs {
		measure, err := model.ResolveMeasure(ref)
		if err != nil {
			return fmt.Errorf("metric view %q measure %q: %w", v.ID, ref, err)
		}
		if measure.Table != v.BaseTable {
			return fmt.Errorf("metric view %q measure %q is owned by %q, want base table %q", v.ID, ref, measure.Table, v.BaseTable)
		}
		v.Measures[ref] = measure
	}
	if _, ok := v.Dimensions[v.Time.DefaultField]; !ok {
		return fmt.Errorf("metric view %q time.default_field %q is not an exposed dimension", v.ID, v.Time.DefaultField)
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
	case sourcereg.KindPath:
		if s.Connection == "" {
			return fmt.Errorf("source %q requires connection", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		connectionSpec, ok := sourcereg.LookupConnection(connection.Kind)
		if !ok || !connectionSpec.AllowsPathSource {
			return fmt.Errorf("source %q path cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
		if connection.Kind == "local" && !sourcereg.IsLocalPath(s.Path) {
			return fmt.Errorf("source %q local connection %q cannot use remote path %q", name, s.Connection, s.Path)
		}
		if connectionSpec.AllowsPathSource && connection.Kind != "local" && sourcereg.IsLocalPath(s.Path) && connection.Scope == "" {
			return fmt.Errorf("source %q remote connection %q requires scope for relative path %q", name, s.Connection, s.Path)
		}
		if s.Format == "" {
			return fmt.Errorf("source %q path requires format", name)
		}
		formatSpec, ok := sourcereg.LookupFormat(s.Format)
		if !ok {
			return fmt.Errorf("source %q has unsupported format %q", name, s.Format)
		}
		if !formatSpec.AllowsOptions && len(s.Options) > 0 {
			return fmt.Errorf("source %q %s path cannot set options", name, s.Format)
		}
	case sourcereg.KindObject:
		if s.Connection == "" {
			return fmt.Errorf("source %q object requires connection", name)
		}
		if s.Format != "" || len(s.Options) > 0 {
			return fmt.Errorf("source %q object cannot set format or options", name)
		}
		connection, ok := connections[s.Connection]
		if !ok {
			return fmt.Errorf("source %q references unknown connection %q", name, s.Connection)
		}
		connectionSpec, ok := sourcereg.LookupConnection(connection.Kind)
		if !ok || !connectionSpec.AllowsObjectSource {
			return fmt.Errorf("source %q object cannot use %s connection %q", name, connection.Kind, s.Connection)
		}
	default:
		return fmt.Errorf("source %q requires exactly one of path or object", name)
	}
	return nil
}

func (c Connection) Validate(name string) (Connection, error) {
	if err := validateSemanticIdentifier(name); err != nil {
		return c, fmt.Errorf("connection %q has invalid name: %w", name, err)
	}
	if c.Kind == "" {
		return c, fmt.Errorf("connection %q requires kind", name)
	}
	connectionSpec, ok := sourcereg.LookupConnection(c.Kind)
	if !ok {
		return c, fmt.Errorf("connection %q has unsupported kind %q", name, c.Kind)
	}
	if connectionSpec.RequiresPath {
		if c.Path == "" {
			return c, fmt.Errorf("connection %q %s requires path", name, c.Kind)
		}
	} else if c.Path != "" && !connectionSpec.AllowsPath {
		return c, fmt.Errorf("connection %q path is only supported for path-backed connections", name)
	}
	auth, err := validateConnectionAuth(name, c, connectionSpec)
	if err != nil {
		return c, err
	}
	c.Auth = auth
	for key := range c.Options {
		if !connectionAllowsOption(connectionSpec, key) {
			return c, fmt.Errorf("connection %q has unsupported option %q", name, key)
		}
	}
	if err := validateConnectionOptions(name, c); err != nil {
		return c, err
	}
	for key := range c.Defaults.Options {
		if err := validateSemanticIdentifier(key); err != nil {
			return c, fmt.Errorf("connection %q default option %q is invalid: %w", name, key, err)
		}
	}
	return c, nil
}

func (s Source) Description() string {
	switch s.Kind() {
	case sourcereg.KindPath:
		if formatSpec, ok := sourcereg.LookupFormat(s.Format); ok && formatSpec.TableLike {
			return s.Format + " table: " + s.Path
		}
		return s.Format + " file: " + s.Path
	case sourcereg.KindObject:
		return "object: " + s.Object
	default:
		return "source"
	}
}

func (s Source) Role() string {
	switch s.Kind() {
	case sourcereg.KindPath:
		return s.Format
	case sourcereg.KindObject:
		return "object"
	default:
		return "source"
	}
}

func (s Source) Kind() string {
	count := 0
	kind := ""
	if s.Path != "" {
		count++
		kind = sourcereg.KindPath
	}
	if s.Object != "" {
		count++
		kind = sourcereg.KindObject
	}
	if count != 1 {
		return ""
	}
	return kind
}

func connectionAllowsOption(connection sourcereg.Connection, option string) bool {
	for _, allowed := range connection.AllowedOptions {
		if option == allowed {
			return true
		}
	}
	return false
}

func validateConnectionOptions(name string, connection Connection) error {
	switch connection.Kind {
	case "quack":
		if !strings.HasPrefix(connection.Path, "quack:") {
			return fmt.Errorf("connection %q quack path must start with quack:", name)
		}
		if value, ok := connection.Options["disable_ssl"]; ok {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("connection %q disable_ssl option must be a boolean", name)
			}
		}
	}
	return nil
}

func validateConnectionAuth(name string, connection Connection, spec sourcereg.Connection) (ConnectionAuth, error) {
	if len(connection.Auth) == 0 {
		if connection.Kind == "ducklake" && duckLakeNeedsAuth(connection) {
			return nil, fmt.Errorf("connection %q ducklake remote path requires auth", name)
		}
		if connection.Kind == "sqlite" && connection.Options["path"] != nil {
			return nil, nil
		}
		if spec.AllowNoAuth {
			return nil, nil
		}
		return nil, fmt.Errorf("connection %q %s requires auth", name, connection.Kind)
	}
	resolved := make(ConnectionAuth, len(connection.Auth))
	for key, value := range connection.Auth {
		if err := validateSemanticIdentifier(key); err != nil {
			return nil, fmt.Errorf("connection %q auth key %q is invalid: %w", name, key, err)
		}
		if !connectionAllowsAuthKey(spec, key) {
			return nil, fmt.Errorf("connection %q has unsupported auth key %q", name, key)
		}
		resolvedValue, err := resolveAuthValue(name, key, value)
		if err != nil {
			return nil, err
		}
		resolved[key] = resolvedValue
	}
	if !connectionHasRequiredAuth(resolved, spec.RequiredAuthSets) {
		return nil, fmt.Errorf("connection %q %s auth is missing required credentials", name, connection.Kind)
	}
	return resolved, nil
}

func connectionAllowsAuthKey(connection sourcereg.Connection, key string) bool {
	for _, allowed := range connection.AuthKeys {
		if key == allowed {
			return true
		}
	}
	return false
}

func connectionHasRequiredAuth(auth ConnectionAuth, requiredSets [][]string) bool {
	if len(requiredSets) == 0 {
		return true
	}
	for _, required := range requiredSets {
		missing := false
		for _, key := range required {
			value, ok := auth[key]
			if !ok || fmt.Sprint(value) == "" {
				missing = true
				break
			}
		}
		if !missing {
			return true
		}
	}
	return false
}

func resolveAuthValue(connectionName, key string, value any) (any, error) {
	switch typed := value.(type) {
	case string:
		if matches := envReferencePattern.FindStringSubmatch(typed); matches != nil {
			envName := matches[1]
			resolved, ok := os.LookupEnv(envName)
			if !ok || resolved == "" {
				return nil, fmt.Errorf("connection %q auth key %q references missing environment variable %s", connectionName, key, envName)
			}
			return resolved, nil
		}
		if typed == "" {
			return nil, fmt.Errorf("connection %q auth key %q cannot be empty", connectionName, key)
		}
		return typed, nil
	case bool, int, int64, float64:
		return typed, nil
	default:
		return nil, fmt.Errorf("connection %q auth key %q has unsupported value type %T", connectionName, key, value)
	}
}

func duckLakeNeedsAuth(connection Connection) bool {
	if connection.Scope != "" && !sourcereg.IsLocalPath(connection.Scope) {
		return true
	}
	if connection.Path != "" && !sourcereg.IsLocalPath(connection.Path) {
		return true
	}
	if dataPath, ok := connection.Options["data_path"]; ok && !sourcereg.IsLocalPath(fmt.Sprint(dataPath)) {
		return true
	}
	return false
}

func validateSemanticIdentifier(value string) error {
	if !semanticIdentifierPattern.MatchString(value) {
		return fmt.Errorf("must match %s", semanticIdentifierPattern.String())
	}
	return nil
}

func (m *Model) TableNames() []string {
	names := make([]string, 0, len(m.Tables))
	for name := range m.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func supportsVisualKind(kind string) bool {
	return kind == "chart" || kind == "kpi"
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
	return renderer == "echarts" || renderer == "html"
}

func rendererSupportsType(renderer, chartType string) bool {
	if renderer == "html" {
		return chartType == "kpi" || chartType == ""
	}
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
	if renderer == "html" {
		return shape == "single_value" && (chartType == "kpi" || chartType == "")
	}
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
