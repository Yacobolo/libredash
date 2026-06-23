package semantic

import (
	"fmt"
	"strings"
)

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
		if len(table.Dimensions) > 0 {
			return MetricDimension{}, fmt.Errorf("unknown dimension %q", fieldName)
		}
		if err := validateSemanticIdentifier(fieldName); err != nil {
			return MetricDimension{}, fmt.Errorf("unknown dimension %q", fieldName)
		}
		dimension = MetricDimension{Expr: fieldName}
	}
	dimension.Field = ref
	dimension.Table = tableName
	dimension.Name = fieldName
	return dimension, nil
}

func (m *Model) ResolveRelationshipEndpoint(ref string) (MetricDimension, error) {
	tableName, fieldName, err := splitSemanticField(ref)
	if err != nil {
		return MetricDimension{}, err
	}
	table, ok := m.Tables[tableName]
	if !ok {
		return MetricDimension{}, fmt.Errorf("unknown table %q", tableName)
	}
	if dimension, ok := table.Dimensions[fieldName]; ok {
		dimension.Field = ref
		dimension.Table = tableName
		dimension.Name = fieldName
		return dimension, nil
	}
	if fieldName == table.PrimaryKey {
		return MetricDimension{Field: ref, Table: tableName, Name: fieldName, Expr: fieldName}, nil
	}
	return MetricDimension{}, fmt.Errorf("unknown relationship endpoint field %q on table %q", fieldName, tableName)
}

func (m *Model) ResolveMeasure(ref string) (MetricMeasure, error) {
	if !strings.Contains(ref, ".") {
		if measure, ok := m.Measures[ref]; ok {
			measure.Field = ref
			measure.Name = ref
			return measure, nil
		}
		return MetricMeasure{}, fmt.Errorf("unknown measure %q", ref)
	}
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
	measure.Table = defaultString(measure.Table, tableName)
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

func (m MetricMeasure) SQLExpression() string {
	if strings.TrimSpace(m.Expression) != "" {
		return m.Expression
	}
	return m.Expr
}
