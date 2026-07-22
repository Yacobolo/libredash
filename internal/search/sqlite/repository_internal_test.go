package sqlite

import "testing"

func TestMatchExpressionCompilesSafePrefixAndPhraseTerms(t *testing.T) {
	tests := map[string]string{
		"orders by region":       `"orders"* AND "region"*`,
		`"orders by region"`:     `"orders by region"`,
		`orders "regional sales`: `"orders"* AND "regional"* AND "sales"*`,
		`the and to`:             "",
	}
	for query, want := range tests {
		if got := matchExpression(query); got != want {
			t.Errorf("matchExpression(%q) = %q, want %q", query, got, want)
		}
	}
}
