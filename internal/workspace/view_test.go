package workspace

import "testing"

func TestWorkspaceViewHelpersFilterAndNavigateAssets(t *testing.T) {
	assets := []AssetView{
		{ID: "model", Type: "semantic_model", Key: "sales", Title: "Sales Model"},
		{ID: "dashboard", Type: "dashboard", Key: "exec", Title: "Executive Sales"},
		{ID: "connection", Type: "connection", Key: "duckdb", Title: "DuckDB"},
		{ID: "source", Type: "source", Key: "orders", Title: "Orders"},
	}
	landing := FilterWorkspaceAssets(assets, "", "")
	if len(landing) != 2 {
		t.Fatalf("landing assets = %#v", landing)
	}
	if filtered := FilterConnectionAssets(assets, "source", "ord"); len(filtered) != 1 || filtered[0].ID != "source" {
		t.Fatalf("filtered connection assets = %#v", filtered)
	}
	edges := []AssetEdgeView{{FromAssetID: "source", ToAssetID: "connection", Type: string(AssetEdgeUsesConnection)}}
	if got := SourceConnectionID("source", edges); got != "connection" {
		t.Fatalf("source connection ID = %q", got)
	}
}

func TestSafeAssetMetaScrubsCredentials(t *testing.T) {
	meta := SafeAssetMeta("connection", `{"kind":"duckdb","auth":{"token":"secret"}}`)
	if meta["auth"] != nil {
		t.Fatalf("auth leaked in meta: %#v", meta)
	}
	if got := meta["credentials_configured"]; got != true {
		t.Fatalf("credentials_configured = %#v", got)
	}
}
