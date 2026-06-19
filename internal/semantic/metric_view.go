package semantic

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

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

func (m *Model) ResolveDimension(ref string) (MetricDimension, error) {
	tableName, fieldName, err := splitSemanticField(ref)
	if err != nil {
		return MetricDimension{}, err
	}
	table, ok := m.Tables[tableName]
	if !ok {
		return MetricDimension{}, fmt.Errorf("unknown table %q", tableName)
	}
	dimension, ok := table.Dimensions[fieldName]
	if !ok {
		return MetricDimension{}, fmt.Errorf("unknown dimension %q", fieldName)
	}
	dimension.Field = ref
	dimension.Table = tableName
	dimension.Name = fieldName
	return dimension, nil
}

func (m *Model) ResolveMeasure(ref string) (MetricMeasure, error) {
	tableName, fieldName, err := splitSemanticField(ref)
	if err != nil {
		return MetricMeasure{}, err
	}
	table, ok := m.Tables[tableName]
	if !ok {
		return MetricMeasure{}, fmt.Errorf("unknown table %q", tableName)
	}
	measure, ok := table.Measures[fieldName]
	if !ok {
		return MetricMeasure{}, fmt.Errorf("unknown measure %q", fieldName)
	}
	measure.Field = ref
	measure.Table = tableName
	measure.Name = fieldName
	return measure, nil
}

func (m *Model) ResolveField(ref string) (MetricDimension, MetricMeasure, string, error) {
	if dimension, err := m.ResolveDimension(ref); err == nil {
		return dimension, MetricMeasure{}, "dimension", nil
	}
	if measure, err := m.ResolveMeasure(ref); err == nil {
		return MetricDimension{}, measure, "measure", nil
	}
	return MetricDimension{}, MetricMeasure{}, "", fmt.Errorf("unknown field %q", ref)
}

func (v *MetricView) ResolveDimensionRef(ref string) (string, MetricDimension, error) {
	if dimension, ok := v.Dimensions[ref]; ok {
		return ref, dimension, nil
	}
	return "", MetricDimension{}, fmt.Errorf("field %q is not exposed", ref)
}

func (v *MetricView) ResolveMeasureRef(ref string) (string, MetricMeasure, error) {
	if measure, ok := v.Measures[ref]; ok {
		return ref, measure, nil
	}
	return "", MetricMeasure{}, fmt.Errorf("field %q is not exposed", ref)
}

func splitSemanticField(ref string) (string, string, error) {
	parts := strings.Split(ref, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("field %q must be qualified as table.field", ref)
	}
	if err := validateSemanticIdentifier(parts[0]); err != nil {
		return "", "", fmt.Errorf("table %q is invalid: %w", parts[0], err)
	}
	if err := validateSemanticIdentifier(parts[1]); err != nil {
		return "", "", fmt.Errorf("field %q is invalid: %w", parts[1], err)
	}
	return parts[0], parts[1], nil
}

func (d MetricDimension) SQLExpression() string {
	if strings.TrimSpace(d.Expr) != "" {
		return d.Expr
	}
	return d.Expression
}
