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
	case "not_equals":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("not_equals filter requires one value")
		}
		return expr + " <> ?", []any{filter.Values[0]}, nil
	case "in", "not_in":
		if len(filter.Values) == 0 {
			return "", nil, nil
		}
		placeholders := make([]string, len(filter.Values))
		args := make([]any, len(filter.Values))
		for i, value := range filter.Values {
			placeholders[i] = "?"
			args[i] = value
		}
		keyword := " IN "
		if filter.Operator == "not_in" {
			keyword = " NOT IN "
		}
		return expr + keyword + "(" + strings.Join(placeholders, ", ") + ")", args, nil
	case "contains":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("contains filter requires one value")
		}
		return "lower(" + expr + ") LIKE lower(?)", []any{"%" + fmt.Sprint(filter.Values[0]) + "%"}, nil
	case "not_contains":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("not_contains filter requires one value")
		}
		return "lower(" + expr + ") NOT LIKE lower(?)", []any{"%" + fmt.Sprint(filter.Values[0]) + "%"}, nil
	case "starts_with":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("starts_with filter requires one value")
		}
		return "lower(" + expr + ") LIKE lower(?)", []any{fmt.Sprint(filter.Values[0]) + "%"}, nil
	case "ends_with":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("ends_with filter requires one value")
		}
		return "lower(" + expr + ") LIKE lower(?)", []any{"%" + fmt.Sprint(filter.Values[0])}, nil
	case "greater_than":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("greater_than filter requires one value")
		}
		return expr + " > ?", []any{filter.Values[0]}, nil
	case "greater_than_or_equal":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("greater_than_or_equal filter requires one value")
		}
		return expr + " >= ?", []any{filter.Values[0]}, nil
	case "less_than":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("less_than filter requires one value")
		}
		return expr + " < ?", []any{filter.Values[0]}, nil
	case "less_than_or_equal":
		if len(filter.Values) != 1 {
			return "", nil, fmt.Errorf("less_than_or_equal filter requires one value")
		}
		return expr + " <= ?", []any{filter.Values[0]}, nil
	case "is_null":
		if len(filter.Values) != 0 {
			return "", nil, fmt.Errorf("is_null filter does not accept values")
		}
		return expr + " IS NULL", nil, nil
	case "is_not_null":
		if len(filter.Values) != 0 {
			return "", nil, fmt.Errorf("is_not_null filter does not accept values")
		}
		return expr + " IS NOT NULL", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported filter operator %q", filter.Operator)
	}
}
