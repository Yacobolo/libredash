package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/Yacobolo/leapview/internal/access"
	"github.com/Yacobolo/leapview/internal/api"
	"github.com/Yacobolo/leapview/internal/workspace"
	"github.com/Yacobolo/leapview/internal/workspace/search"
)

func TestSearchIndexCacheReusesOnlyCurrentServingState(t *testing.T) {
	cache := &SearchIndexCache{}
	builds := 0
	build := func() *search.Index {
		builds++
		return search.NewIndex([]search.Document{{ID: "orders", Type: "dashboard", Name: "Orders"}})
	}
	first := cache.index("sales", "prod", "state-1", build)
	second := cache.index("sales", "prod", "state-1", build)
	third := cache.index("sales", "prod", "state-2", build)
	if first != second || first == third || builds != 2 {
		t.Fatalf("cache pointers = (%p, %p, %p), builds = %d; want reuse then rebuild", first, second, third, builds)
	}
}

func TestFilterReadableSearchResultsStopsAtLimit(t *testing.T) {
	rows := []api.SearchResult{
		{ID: "one", Name: "One"},
		{ID: "two", Name: "Two"},
		{ID: "three", Name: "Three"},
	}

	got, err := (Handler{}).filterReadableSearchResults(httptest.NewRequest(nethttp.MethodGet, "/", nil), "workspace", rows, 2)
	if err != nil {
		t.Fatalf("filterReadableSearchResults: %v", err)
	}
	if len(got) != 2 || got[0].ID != "one" || got[1].ID != "two" {
		t.Fatalf("results = %#v, want first two rows", got)
	}
}

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
