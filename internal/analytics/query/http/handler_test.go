package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/ipc"
)

func TestSemanticQueryResponseUsesTypedColumnsAndPrecisionSafePositionalRows(t *testing.T) {
	response := semanticQueryResponse([]string{"order_id", "amount", "active"}, reportdef.QueryRows{{
		"order_id": int64(9007199254740993), "amount": 12.5, "active": true,
	}}, 100, 0, "query-1", "state-1")
	if len(response.Columns) != 3 || response.Columns[0].Type != "int64" || response.Columns[1].Type != "float64" || response.Columns[2].Type != "boolean" {
		t.Fatalf("columns = %#v", response.Columns)
	}
	if len(response.Rows) != 1 || response.Rows[0][0] != "9007199254740993" || response.Rows[0][1] != "12.5" || response.Rows[0][2] != "true" {
		t.Fatalf("rows = %#v", response.Rows)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(encoded) == "" || response.QueryID != "query-1" || response.ServingSnapshot != "state-1" {
		t.Fatalf("response = %s %#v", encoded, response)
	}
}

func TestIndexCursorIsSignedBoundAndExpiring(t *testing.T) {
	token := encodeIndexCursor(25, "query-a", "snapshot-a")
	if offset, err := decodeIndexCursor(token, "query-a", "snapshot-a"); err != nil || offset != 25 {
		t.Fatalf("decode cursor offset=%d err=%v", offset, err)
	}
	if _, err := decodeIndexCursor(token+"x", "query-a", "snapshot-a"); err == nil {
		t.Fatal("tampered cursor was accepted")
	}
	if _, err := decodeIndexCursor(token, "query-b", "snapshot-a"); err == nil {
		t.Fatal("cursor was accepted for another request")
	}
	if _, err := decodeIndexCursor(token, "query-a", "snapshot-b"); !errors.Is(err, errCursorSnapshotUnavailable) {
		t.Fatalf("snapshot mismatch error = %v", err)
	}
	expired := encodeIndexCursorValue(indexCursor{Offset: 25, Scope: "query-a", Snapshot: "snapshot-a", Expires: time.Now().Add(-time.Second).Unix()})
	if _, err := decodeIndexCursor(expired, "query-a", "snapshot-a"); err == nil {
		t.Fatal("expired cursor was accepted")
	}
}

func TestRequestCursorScopeIgnoresPageTokenButBindsNormalizedRequest(t *testing.T) {
	first := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query?limit=100&pageToken=one", nil)
	second := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query?pageToken=two&limit=100", nil)
	if requestCursorScope(first, map[string]any{"field": "revenue"}) != requestCursorScope(second, map[string]any{"field": "revenue"}) {
		t.Fatal("page token changed normalized request scope")
	}
	changed := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/sales/semantic-models/sales/query?limit=200", nil)
	if requestCursorScope(first, map[string]any{"field": "revenue"}) == requestCursorScope(changed, map[string]any{"field": "revenue"}) {
		t.Fatal("query shape did not change request scope")
	}
}

func TestSemanticArrowMatchesJSONRowsAndCarriesMetadata(t *testing.T) {
	response := semanticQueryResponse([]string{"id", "amount"}, reportdef.QueryRows{{"id": int64(9007199254740993), "amount": 12.5}}, 100, 0, "query-a", "state-a")
	encoded, err := encodeSemanticArrow(response)
	if err != nil {
		t.Fatalf("encode Arrow: %v", err)
	}
	reader, err := ipc.NewReader(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("open Arrow: %v", err)
	}
	defer reader.Release()
	metadata := reader.Schema().Metadata()
	if value, ok := metadata.GetValue("leapview.query_id"); !ok || value != "query-a" {
		t.Fatalf("query metadata = %q, %v", value, ok)
	}
	if !reader.Next() {
		t.Fatalf("read Arrow record: %v", reader.Err())
	}
	record := reader.Record()
	if record.NumRows() != 1 || record.NumCols() != 2 {
		t.Fatalf("record shape = %dx%d", record.NumRows(), record.NumCols())
	}
	if got := record.Column(0).(*array.String).Value(0); got != response.Rows[0][0] {
		t.Fatalf("Arrow id = %q, JSON = %q", got, response.Rows[0][0])
	}
}

func TestSemanticRelationshipDTOParsesTypedEndpoints(t *testing.T) {
	got, err := semanticRelationshipDTO(semanticmodel.Relationship{
		ID:          "orders_customers",
		From:        "orders.customer_id",
		To:          "customers.customer_id",
		Cardinality: "many_to_one",
	})
	if err != nil {
		t.Fatalf("semanticRelationshipDTO: %v", err)
	}
	if got.ID != "orders_customers" || got.FromDataset != "orders" || got.FromField != "customer_id" || got.ToDataset != "customers" || got.ToField != "customer_id" || got.Cardinality != "many_to_one" || !got.Active {
		t.Fatalf("unexpected relationship: %#v", got)
	}
}

func TestSemanticRelationshipDTORejectsMalformedEndpoint(t *testing.T) {
	if _, err := semanticRelationshipDTO(semanticmodel.Relationship{ID: "broken", From: "orders", To: "customers.id"}); err == nil {
		t.Fatal("expected malformed endpoint error")
	}
}
