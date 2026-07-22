package query

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteIdent(value string) (string, error) {
	if !identifierPattern.MatchString(value) {
		return "", fmt.Errorf("invalid identifier %q", value)
	}
	return value, nil
}

func applyAliases(expr string, aliases map[string]tableAlias, fallbackAlias string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return expr
	}
	if identifierPattern.MatchString(expr) {
		return fallbackAlias + "." + expr
	}
	for table, alias := range aliases {
		expr = regexp.MustCompile(`\b`+regexp.QuoteMeta(table)+`\.`).ReplaceAllString(expr, alias.Alias+".")
	}
	expr = strings.ReplaceAll(expr, "{alias}", fallbackAlias)
	return expr
}

func joinSQL(planner *Planner, base string, aliases map[string]tableAlias) (string, error) {
	baseRelation, err := planner.physicalTable(base)
	if err != nil {
		return "", err
	}
	model := planner.Model
	parts := []string{baseRelation + " t0"}
	joinAliases := make([]tableAlias, 0, len(aliases)-1)
	for table, alias := range aliases {
		if table != base {
			joinAliases = append(joinAliases, alias)
		}
	}
	sort.Slice(joinAliases, func(i, j int) bool {
		if len(joinAliases[i].Path) != len(joinAliases[j].Path) {
			return len(joinAliases[i].Path) < len(joinAliases[j].Path)
		}
		return joinAliases[i].Alias < joinAliases[j].Alias
	})
	for _, alias := range joinAliases {
		if len(alias.Path) == 0 {
			continue
		}
		relationship := alias.Path[len(alias.Path)-1]
		fromEndpoint, err := model.ResolveRelationshipEndpoint(relationship.From)
		if err != nil {
			return "", err
		}
		toEndpoint, err := model.ResolveRelationshipEndpoint(relationship.To)
		if err != nil {
			return "", err
		}
		rightRelation, err := planner.physicalTable(alias.Table)
		if err != nil {
			return "", err
		}
		leftEndpoint := fromEndpoint
		rightEndpoint := toEndpoint
		if alias.Table == fromEndpoint.Table && relationship.Cardinality == "one_to_one" {
			leftEndpoint = toEndpoint
			rightEndpoint = fromEndpoint
		}
		left, ok := aliases[leftEndpoint.Table]
		if !ok {
			return "", fmt.Errorf("missing relationship alias for %q", leftEndpoint.Table)
		}
		right, ok := aliases[rightEndpoint.Table]
		if !ok {
			return "", fmt.Errorf("missing relationship alias for %q", rightEndpoint.Table)
		}
		if right.Table != alias.Table {
			return "", fmt.Errorf("relationship path to %q ends at %q", alias.Table, right.Table)
		}
		leftExpr := applyAliases(leftEndpoint.SQLExpression(), aliases, left.Alias)
		rightExpr := applyAliases(rightEndpoint.SQLExpression(), aliases, right.Alias)
		parts = append(parts, fmt.Sprintf("LEFT JOIN %s %s ON %s = %s", rightRelation, alias.Alias, leftExpr, rightExpr))
	}
	return strings.Join(parts, "\n"), nil
}

func joinPathSQL(planner *Planner, aliases pathAliasSet) (string, error) {
	baseRelation, err := planner.physicalTable(aliases.BaseTable)
	if err != nil {
		return "", err
	}
	baseAlias, ok := aliases.ByPath[""]
	if !ok {
		return "", fmt.Errorf("missing base alias for fact %q", aliases.BaseTable)
	}
	model := planner.Model
	parts := []string{baseRelation + " " + baseAlias.Alias}
	for _, alias := range aliases.Ordered {
		if len(alias.Path) == 0 {
			return "", fmt.Errorf("join alias %q has no relationship path", alias.Alias)
		}
		parentPath := alias.Path[:len(alias.Path)-1]
		parent, ok := aliases.ByPath[relationshipPathSignature(parentPath)]
		if !ok {
			return "", fmt.Errorf("missing parent alias for relationship path %q", relationshipPathSignature(alias.Path))
		}
		relationship := alias.Path[len(alias.Path)-1]
		fromEndpoint, err := model.ResolveRelationshipEndpoint(relationship.From)
		if err != nil {
			return "", err
		}
		toEndpoint, err := model.ResolveRelationshipEndpoint(relationship.To)
		if err != nil {
			return "", err
		}
		leftEndpoint := fromEndpoint
		rightEndpoint := toEndpoint
		switch {
		case parent.Table == fromEndpoint.Table && alias.Table == toEndpoint.Table:
		case relationship.Cardinality == "one_to_one" && parent.Table == toEndpoint.Table && alias.Table == fromEndpoint.Table:
			leftEndpoint = toEndpoint
			rightEndpoint = fromEndpoint
		default:
			return "", fmt.Errorf("relationship path %q cannot join %q to %q", relationshipPathSignature(alias.Path), parent.Table, alias.Table)
		}
		rightRelation, err := planner.physicalTable(alias.Table)
		if err != nil {
			return "", err
		}
		leftAliases, err := aliases.context(parentPath)
		if err != nil {
			return "", err
		}
		rightAliases, err := aliases.context(alias.Path)
		if err != nil {
			return "", err
		}
		leftExpr := applyAliases(leftEndpoint.SQLExpression(), leftAliases, parent.Alias)
		rightExpr := applyAliases(rightEndpoint.SQLExpression(), rightAliases, alias.Alias)
		parts = append(parts, fmt.Sprintf("LEFT JOIN %s %s ON %s = %s", rightRelation, alias.Alias, leftExpr, rightExpr))
	}
	return strings.Join(parts, "\n"), nil
}

func dimensionExpr(dimension semanticmodel.MetricDimension, aliases map[string]tableAlias) string {
	alias := aliases[dimension.Table].Alias
	return applyAliases(dimension.SQLExpression(), aliases, alias)
}

func dimensionExprForPath(dimension semanticmodel.MetricDimension, aliases pathAliasSet, path []semanticmodel.Relationship) (string, error) {
	context, err := aliases.context(path)
	if err != nil {
		return "", err
	}
	alias, ok := context[dimension.Table]
	if !ok {
		return "", fmt.Errorf("relationship path %q does not expose table %q", relationshipPathSignature(path), dimension.Table)
	}
	return applyAliases(dimension.SQLExpression(), context, alias.Alias), nil
}

func dimensionWhereExpr(dimension semanticmodel.MetricDimension, aliases map[string]tableAlias) string {
	return ""
}

func measureExpr(model *semanticmodel.Model, measure ResolvedMeasure, aliases map[string]tableAlias) (string, error) {
	input, err := rawMeasureExpr(model, measure, aliases)
	if err != nil && measure.Aggregation != "count" {
		return "", err
	}
	expr := ""
	switch measure.Aggregation {
	case "count":
		expr = "COUNT(*)"
	case "count_distinct":
		expr = "COUNT(DISTINCT " + input + ")"
	case "sum", "avg", "min", "max":
		expr = strings.ToUpper(measure.Aggregation) + "(" + input + ")"
	default:
		return "", fmt.Errorf("measure %q has unsupported aggregation %q", measure.Name, measure.Aggregation)
	}
	return expr, nil
}

func rawMeasureExpr(model *semanticmodel.Model, measure ResolvedMeasure, aliases map[string]tableAlias) (string, error) {
	if measure.InputField != "" {
		dimension, err := model.ResolveDimension(measure.InputField)
		if err != nil {
			return "", err
		}
		return dimensionExpr(dimension, aliases), nil
	}
	if measure.InputExpr == "" {
		return "", fmt.Errorf("measure %q has no raw input", measure.Name)
	}
	var expression semanticmodel.Expression
	if measure.InputExpression != nil {
		expression = *measure.InputExpression
	} else {
		var err error
		expression, err = semanticmodel.ParseExpression(measure.InputExpr)
		if err != nil {
			return "", err
		}
	}
	return expression.SQL(func(ref string) (string, error) {
		dimension, err := model.ResolveDimension(ref)
		if err != nil {
			return "", err
		}
		return dimensionExpr(dimension, aliases), nil
	})
}
