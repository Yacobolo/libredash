package query

import "testing"

func TestFilterSQLValidatesUnaryOperatorValues(t *testing.T) {
	for _, operator := range []string{"equals", "contains", "not_contains", "starts_with", "greater_than_or_equal", "less_than"} {
		if _, _, err := filterSQL("value", Filter{Operator: operator}); err == nil {
			t.Fatalf("filterSQL(%q) accepted an empty value list", operator)
		}
	}
}

func TestFilterSQLSupportsNotContains(t *testing.T) {
	sql, args, err := filterSQL("value", Filter{Operator: "not_contains", Values: []any{"internal"}})
	if err != nil {
		t.Fatal(err)
	}
	if sql != "lower(value) NOT LIKE lower(?)" || len(args) != 1 || args[0] != "%internal%" {
		t.Fatalf("filter = %q %#v", sql, args)
	}
}

func TestFilterSQLSupportsNullSelection(t *testing.T) {
	for _, test := range []struct {
		operator string
		wantSQL  string
	}{
		{operator: "is_null", wantSQL: "value IS NULL"},
		{operator: "is_not_null", wantSQL: "value IS NOT NULL"},
	} {
		sql, args, err := filterSQL("value", Filter{Operator: test.operator})
		if err != nil {
			t.Fatalf("filterSQL(%q): %v", test.operator, err)
		}
		if sql != test.wantSQL || len(args) != 0 {
			t.Fatalf("filterSQL(%q) = %q %#v, want %q with no args", test.operator, sql, args, test.wantSQL)
		}
		if _, _, err := filterSQL("value", Filter{Operator: test.operator, Values: []any{"unexpected"}}); err == nil {
			t.Fatalf("filterSQL(%q) accepted a value", test.operator)
		}
	}
}

func TestFilterSQLSupportsCompleteDashboardFilterOperatorSet(t *testing.T) {
	tests := []struct {
		operator string
		values   []any
		wantSQL  string
		wantArgs []any
	}{
		{operator: "not_equals", values: []any{"closed"}, wantSQL: "value <> ?", wantArgs: []any{"closed"}},
		{operator: "ends_with", values: []any{"son"}, wantSQL: "lower(value) LIKE lower(?)", wantArgs: []any{"%son"}},
		{operator: "greater_than", values: []any{10}, wantSQL: "value > ?", wantArgs: []any{10}},
		{operator: "less_than_or_equal", values: []any{20}, wantSQL: "value <= ?", wantArgs: []any{20}},
		{operator: "not_in", values: []any{"CA", "WA"}, wantSQL: "value NOT IN (?, ?)", wantArgs: []any{"CA", "WA"}},
	}
	for _, test := range tests {
		t.Run(test.operator, func(t *testing.T) {
			sql, args, err := filterSQL("value", Filter{Operator: test.operator, Values: test.values})
			if err != nil {
				t.Fatal(err)
			}
			if sql != test.wantSQL {
				t.Fatalf("SQL = %q, want %q", sql, test.wantSQL)
			}
			if len(args) != len(test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
			}
			for index := range args {
				if args[index] != test.wantArgs[index] {
					t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
				}
			}
		})
	}
}
