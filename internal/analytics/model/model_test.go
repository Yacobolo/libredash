package model

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedConnectionRejectsAuthoredPhysicalLocation(t *testing.T) {
	for _, connection := range []Connection{
		{Kind: "managed", Root: "/server/revision"},
		{Kind: "managed", Scope: "s3://private-bucket/revision"},
	} {
		if _, err := connection.Validate("olist"); err == nil || !strings.Contains(err.Error(), "physical location") {
			t.Fatalf("Validate() error = %v, want managed physical location rejection", err)
		}
	}
}

func TestConnectionRejectsRemovedLocalKind(t *testing.T) {
	_, err := (Connection{Kind: "local"}).Validate("files")
	if err == nil || !strings.Contains(err.Error(), `unsupported kind "local"`) {
		t.Fatalf("Validate() error = %v, want unsupported local kind", err)
	}
}

func TestManagedSourceRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	connections := map[string]Connection{"olist": {Kind: "managed"}}
	for _, value := range []string{filepath.Join(string(filepath.Separator), "orders.csv"), "../orders.csv"} {
		source := Source{Connection: "olist", Path: value, Format: "csv"}
		if err := source.Validate("orders", connections); err == nil {
			t.Fatalf("Validate(%q) error = nil, want unsafe managed path rejection", value)
		}
	}
}

func TestValidateRejectsAuthoredSourceReads(t *testing.T) {
	model := &Model{
		Name:        "test",
		Connections: map[string]Connection{"files": {Kind: "managed"}},
		Sources: map[string]Source{
			"orders": {Connection: "files", Path: "orders.csv", Format: "csv"},
		},
		BaseTable: "orders",
		Tables: map[string]Table{
			"orders": {
				Sources:     []string{"orders"},
				SourceReads: map[string][]string{"orders": {"order_id"}},
				PrimaryKey:  "order_id",
				Dimensions:  map[string]MetricDimension{"order_id": {Label: "Order ID"}},
				Transform:   Transform{SQL: "SELECT order_id FROM source.orders"},
			},
		},
	}

	err := model.Validate()
	if err == nil || !strings.Contains(err.Error(), "source_reads is no longer supported") {
		t.Fatalf("Validate() error = %v, want source_reads rejection", err)
	}
}
