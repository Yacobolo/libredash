package query

import (
	"fmt"
	"strings"
)

func filterSQL(expr string, filter Filter) (string, []any, error) {
	switch filter.Operator {
	case "", "equals":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("equals filter requires one value")
		}
		return expr + " = ?", []any{filter.Values[0]}, nil
	case "in":
		if len(filter.Values) == 0 {
			return "", nil, nil
		}
		placeholders := make([]string, len(filter.Values))
		args := make([]any, len(filter.Values))
		for i, value := range filter.Values {
			placeholders[i] = "?"
			args[i] = value
		}
		return expr + " IN (" + strings.Join(placeholders, ", ") + ")", args, nil
	case "contains":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("contains filter requires one value")
		}
		return "lower(" + expr + ") LIKE lower(?)", []any{"%" + fmt.Sprint(filter.Values[0]) + "%"}, nil
	case "starts_with":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("starts_with filter requires one value")
		}
		return "lower(" + expr + ") LIKE lower(?)", []any{fmt.Sprint(filter.Values[0]) + "%"}, nil
	case "greater_than_or_equal":
		return expr + " >= ?", []any{filter.Values[0]}, nil
	case "less_than":
		return expr + " < ?", []any{filter.Values[0]}, nil
	default:
		return "", nil, fmt.Errorf("unsupported filter operator %q", filter.Operator)
	}
}
