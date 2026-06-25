package app

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestSafeAssetMetaKeepsModelTableDefinition(t *testing.T) {
	meta := workspace.SafeAssetMeta("model_table", `{
		"Source": "orders",
		"Sources": ["orders", "payments"],
		"SourceDependencies": ["orders", "payments"],
		"Transform": {"SQL": "SELECT * FROM source.orders"},
		"PrimaryKey": "order_id",
		"Grain": "order_id",
		"Dimensions": {"order_id": {"Expr": "order_id"}},
		"Measures": {"revenue": {"Expression": "SUM(revenue)"}},
		"Auth": {"token": "secret"}
	}`)

	for _, key := range []string{"Source", "Sources", "SourceDependencies", "Transform", "PrimaryKey", "Grain", "Dimensions"} {
		if _, ok := meta[key]; !ok {
			t.Fatalf("model table meta missing %s: %#v", key, meta)
		}
	}
	if _, ok := meta["Measures"]; ok {
		t.Fatalf("model table meta should not expose measures: %#v", meta)
	}
	if _, ok := meta["Auth"]; ok {
		t.Fatalf("model table meta should not expose auth: %#v", meta)
	}
}
