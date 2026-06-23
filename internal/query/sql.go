package query

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/semantic"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var aggregateWrapperPattern = regexp.MustCompile(`(?is)^\s*(?:AVG|SUM|MIN|MAX|MEDIAN|QUANTILE_CONT)\s*\((.+?)(?:,\s*[-0-9.]+)?\)\s*$`)

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

func joinSQL(model *semantic.Model, base string, aliases map[string]tableAlias) (string, error) {
	baseIdent, err := quoteIdent(base)
	if err != nil {
		return "", err
	}
	parts := []string{"model." + baseIdent + " t0"}
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
		rightIdent, err := quoteIdent(alias.Table)
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
		parts = append(parts, fmt.Sprintf("LEFT JOIN model.%s %s ON %s = %s", rightIdent, alias.Alias, leftExpr, rightExpr))
	}
	return strings.Join(parts, "\n"), nil
}

func dimensionExpr(dimension semantic.MetricDimension, aliases map[string]tableAlias) string {
	alias := aliases[dimension.Table].Alias
	return applyAliases(dimension.SQLExpression(), aliases, alias)
}

func dimensionWhereExpr(dimension semantic.MetricDimension, aliases map[string]tableAlias) string {
	if strings.TrimSpace(dimension.Where) == "" {
		return ""
	}
	alias := aliases[dimension.Table].Alias
	return applyAliases(dimension.Where, aliases, alias)
}

func measureExpr(measure ResolvedMeasure, aliases map[string]tableAlias) string {
	alias := aliases[measure.Table].Alias
	return applyAliases(measure.SQLExpression(), aliases, alias)
}

func rawMeasureExpr(measure ResolvedMeasure, aliases map[string]tableAlias) (string, error) {
	expr := strings.TrimSpace(measure.SQLExpression())
	if expr == "" {
		return "", fmt.Errorf("measure %q is missing expression", measure.Label)
	}
	if matches := aggregateWrapperPattern.FindStringSubmatch(expr); len(matches) == 2 {
		expr = strings.TrimSpace(matches[1])
	} else if strings.Contains(expr, "(") {
		return "", fmt.Errorf("measure %q cannot be used as a raw value expression", measure.Label)
	}
	alias := aliases[measure.Table].Alias
	return applyAliases(expr, aliases, alias), nil
}
