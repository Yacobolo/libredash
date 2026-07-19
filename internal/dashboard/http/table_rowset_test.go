package http

import (
	"bytes"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
)

func TestDashboardTableRowsetIsTypedPrecisionSafeAndCursorPaged(t *testing.T) {
	table := dashboard.Table{
		Title: "Orders", AvailableRows: 2,
		Columns: []dashboard.TableColumn{{Key: "order_id"}, {Key: "amount"}},
		Blocks:  map[string]dashboard.TableBlock{"a": {Rows: []map[string]any{{"order_id": int64(9007199254740993), "amount": 12.5}}}},
	}
	response := dashboardTableRowset("orders", table, "a", 0, 1, "scope-a", "snapshot-a")
	if len(response.Columns) != 2 || response.Columns[0].Type != "int64" || response.Columns[1].Type != "float64" {
		t.Fatalf("columns = %#v", response.Columns)
	}
	if len(response.Rows) != 1 || response.Rows[0][0] != "9007199254740993" || response.Page.NextCursor == "" {
		t.Fatalf("rowset = %#v", response)
	}
	if offset, err := decodeIndexCursor(response.Page.NextCursor, "scope-a", "snapshot-a"); err != nil || offset != 1 {
		t.Fatalf("cursor offset=%d err=%v", offset, err)
	}
}

func TestDashboardTableArrowMatchesJSONAndCarriesSnapshotMetadata(t *testing.T) {
	table := dashboard.Table{
		Title: "Orders", AvailableRows: 1,
		Columns: []dashboard.TableColumn{{Key: "order_id"}},
		Blocks:  map[string]dashboard.TableBlock{"a": {Rows: []map[string]any{{"order_id": int64(9007199254740993)}}}},
	}
	response := dashboardTableRowset("orders", table, "a", 0, 100, "scope-a", "snapshot-a")
	payload, err := encodeDashboardTableArrow(response)
	if err != nil {
		t.Fatalf("encode Arrow: %v", err)
	}
	reader, err := ipc.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("open Arrow: %v", err)
	}
	defer reader.Release()
	if snapshot, ok := reader.Schema().Metadata().GetValue("libredash.serving_snapshot"); !ok || snapshot != "snapshot-a" {
		t.Fatalf("snapshot metadata = %q, %v", snapshot, ok)
	}
	if !reader.Next() {
		t.Fatalf("read record: %v", reader.Err())
	}
	if got := reader.Record().Column(0).(*array.String).Value(0); got != response.Rows[0][0] {
		t.Fatalf("Arrow value=%q JSON=%q", got, response.Rows[0][0])
	}
}
