package http

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/workspace"
)

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
