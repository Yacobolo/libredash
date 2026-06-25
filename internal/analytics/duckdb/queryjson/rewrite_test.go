package queryjson

import (
	"strings"
	"testing"
)

func TestRewriteSourceRefsRewritesOnlyExecutableTableRefs(t *testing.T) {
	sql := `
WITH revenue AS (
	SELECT order_id FROM source.payments
)
SELECT 'source.orders' AS literal, o.order_id
FROM source.orders AS o
JOIN "source".payments p USING (order_id)
-- source.orders in comment
`
	refs := []TableRef{
		{Schema: "source", Table: "payments", QueryLocation: strings.Index(sql, "source.payments")},
		{Schema: "source", Table: "orders", QueryLocation: strings.Index(sql, "source.orders AS o")},
		{Schema: "source", Table: "payments", QueryLocation: strings.Index(sql, `"source".payments`)},
	}
	got, err := RewriteSourceRefs(sql, refs, map[string]string{
		"orders":   "(SELECT order_id FROM read_csv('orders.csv'))",
		"payments": "(SELECT order_id FROM read_csv('payments.csv'))",
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains := "'source.orders'"; !strings.Contains(got, contains) {
		t.Fatalf("rewritten SQL = %s, want literal %q preserved", got, contains)
	}
	if contains := "-- source.orders in comment"; !strings.Contains(got, contains) {
		t.Fatalf("rewritten SQL = %s, want comment preserved", got)
	}
	if strings.Contains(got, "FROM source.orders") || strings.Contains(got, `"source".payments`) {
		t.Fatalf("rewritten SQL still contains executable source refs: %s", got)
	}
	if !strings.Contains(got, "FROM (SELECT order_id FROM read_csv('orders.csv')) AS o") {
		t.Fatalf("rewritten SQL = %s, want orders relation replacement", got)
	}
	if !strings.Contains(got, `JOIN (SELECT order_id FROM read_csv('payments.csv')) p`) {
		t.Fatalf("rewritten SQL = %s, want payments relation replacement", got)
	}
}

func TestRewriteSourceRefsRejectsMissingReplacement(t *testing.T) {
	sql := `SELECT * FROM source.orders`
	_, err := RewriteSourceRefs(sql, []TableRef{{Schema: "source", Table: "orders", QueryLocation: strings.Index(sql, "source.orders")}}, map[string]string{})
	if err == nil || err.Error() != `no replacement for source "orders"` {
		t.Fatalf("RewriteSourceRefs() error = %v, want missing replacement", err)
	}
}

func TestRewriteSourceRefsRejectsInvalidLocation(t *testing.T) {
	_, err := RewriteSourceRefs("SELECT * FROM source.orders", []TableRef{{Schema: "source", Table: "orders", QueryLocation: -1}}, map[string]string{"orders": "orders"})
	if err == nil {
		t.Fatal("RewriteSourceRefs() unexpectedly succeeded")
	}
}
