package model

import (
	"fmt"
	"regexp"
)

var discoveredFieldRefPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\b`)

func (m *Model) ValidateDiscoveredSchemas() error {
	if m == nil {
		return fmt.Errorf("semantic model is required")
	}
	if err := m.ValidateDiscoveredSourceSchemas(); err != nil {
		return err
	}
	for tableName, table := range m.Tables {
		columns := map[string]struct{}{}
		for _, column := range table.Schema.Columns {
			columns[column.Name] = struct{}{}
		}
		if len(columns) == 0 {
			return fmt.Errorf("model table %q has no discovered schema", tableName)
		}
		if _, ok := columns[table.PrimaryKey]; !ok {
			return fmt.Errorf("model table %q primary_key %q is not in discovered schema", tableName, table.PrimaryKey)
		}
		for field := range table.Dimensions {
			if _, ok := columns[field]; !ok {
				return fmt.Errorf("model table %q field %q is not in discovered schema", tableName, field)
			}
		}
	}
	for measureName, measure := range m.Measures {
		refs := []string{measure.Input.Field}
		if measure.Input.Expression != "" {
			expression, err := ParseExpression(measure.Input.Expression)
			if err != nil {
				return fmt.Errorf("measure %q input expression: %w", measureName, err)
			}
			refs = append(refs, expression.References()...)
		}
		for _, ref := range refs {
			if ref == "" {
				continue
			}
			if _, err := m.ResolveDimension(ref); err != nil {
				return fmt.Errorf("measure %q references unknown field %q", measureName, ref)
			}
		}
	}
	return nil
}

func (m *Model) ValidateDiscoveredSourceSchemas() error {
	if m == nil {
		return fmt.Errorf("semantic model is required")
	}
	for sourceName, source := range m.Sources {
		columns := map[string]struct{}{}
		for _, column := range source.Schema.Columns {
			columns[column.Name] = struct{}{}
		}
		if len(columns) == 0 {
			return fmt.Errorf("source %q has no discovered schema", sourceName)
		}
		for field := range source.Fields {
			if _, ok := columns[field]; !ok {
				return fmt.Errorf("source %q field %q is not in discovered schema", sourceName, field)
			}
		}
	}
	return nil
}

func ExpressionFieldRefs(expression string) []string {
	matches := discoveredFieldRefPattern.FindAllStringSubmatch(expression, -1)
	seen := map[string]struct{}{}
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		ref := match[1] + "." + match[2]
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func discoveredFieldRefs(expression string) []string {
	return ExpressionFieldRefs(expression)
}
