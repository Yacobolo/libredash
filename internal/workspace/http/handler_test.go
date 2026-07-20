package http

import (
	"testing"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/api"
	"github.com/Yacobolo/libredash/internal/workspace"
)

func TestSearchResultObjectUsesRegisteredSemanticFieldIdentity(t *testing.T) {
	tests := []struct {
		name   string
		row    api.SearchResult
		object access.ObjectRef
	}{
		{
			name: "dimension",
			row:  api.SearchResult{ModelID: "sales", DatasetID: "orders", FieldID: "orders.state"},
			object: access.ItemObjectWithParent(
				access.SecurableSemanticField, "commerce", "sales/orders.state",
				access.ItemObjectWithParent(access.SecurableSemanticModel, "commerce", "sales", access.WorkspaceObject("commerce")),
			),
		},
		{
			name: "measure",
			row:  api.SearchResult{ModelID: "sales", DatasetID: "orders", FieldID: "order_count"},
			object: access.ItemObjectWithParent(
				access.SecurableSemanticField, "commerce", "sales/order_count",
				access.ItemObjectWithParent(access.SecurableSemanticModel, "commerce", "sales", access.WorkspaceObject("commerce")),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object, ok := searchResultObject("commerce", test.row)
			if !ok || object != test.object {
				t.Fatalf("object = %#v, %t; want %#v, true", object, ok, test.object)
			}
		})
	}
}

func TestAssetDTOUsesLogicalIDAndTypedPayload(t *testing.T) {
	asset, err := workspace.NewAsset(
		workspace.WorkspaceID("test"),
		workspace.ServingStateID("deploy_a"),
		workspace.AssetTypeVisual,
		"executive-sales.orders",
		workspace.AssetID("dashboard:executive-sales"),
		"Orders",
		"Orders visual",
		"visual.v1",
		map[string]any{"query_kind": "aggregate"},
	)
	if err != nil {
		t.Fatalf("asset: %v", err)
	}

	catalog, err := workspace.DecodeAssetCatalog(workspace.AssetGraph{Assets: []workspace.Asset{asset}})
	if err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	dto := apiAssetDTOs([]workspace.AssetView{workspace.AssetViewFromCatalogRecord(catalog.Assets[0])})[0]
	if dto.ID != "visual:executive-sales.orders" || dto.SnapshotID == "" || dto.SnapshotID == dto.ID {
		t.Fatalf("asset identity = %#v", dto)
	}
	if dto.ParentID != "dashboard:executive-sales" || dto.PayloadSchema != "visual.v1" || dto.Payload["query_kind"] != "aggregate" {
		t.Fatalf("asset dto = %#v", dto)
	}
}
