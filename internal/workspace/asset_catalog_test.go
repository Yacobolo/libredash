package workspace

import (
	"context"
	"testing"
)

func TestAssetCatalogServiceReadsActiveDeploymentBeforeRuntimeFallback(t *testing.T) {
	ctx := context.Background()
	persisted := mustCatalogTestAsset(t, "test", "dep_active", AssetTypeDashboard, "sales")
	runtime := mustCatalogTestAsset(t, "test", "local", AssetTypeDashboard, "local-sales")
	service := NewAssetCatalogService(fakeCatalogRepo{
		graph: AssetGraph{Assets: []Asset{persisted}},
		ok:    true,
	}).WithRuntimeProvider(fakeRuntimeAssetGraphProvider{
		assets: []Asset{runtime},
		ok:     true,
	})

	catalog, ok, err := service.ActiveAssetCatalog(ctx, "test", "dev")
	if err != nil {
		t.Fatalf("ActiveAssetCatalog() error = %v", err)
	}
	if !ok {
		t.Fatal("ActiveAssetCatalog() ok = false")
	}
	if len(catalog.Assets) != 1 || catalog.Assets[0].ID != persisted.ID {
		t.Fatalf("catalog assets = %#v, want persisted asset", catalog.Assets)
	}
}

func TestAssetCatalogServiceFallsBackToRuntimeGraph(t *testing.T) {
	ctx := context.Background()
	runtime := mustCatalogTestAsset(t, "test", "local", AssetTypeDashboard, "local-sales")
	service := NewAssetCatalogService(fakeCatalogRepo{}).WithRuntimeProvider(fakeRuntimeAssetGraphProvider{
		assets: []Asset{runtime},
		ok:     true,
	})

	catalog, ok, err := service.ActiveAssetCatalog(ctx, "test", "dev")
	if err != nil {
		t.Fatalf("ActiveAssetCatalog() error = %v", err)
	}
	if !ok {
		t.Fatal("ActiveAssetCatalog() ok = false")
	}
	if len(catalog.Assets) != 1 || catalog.Assets[0].ID != runtime.ID || catalog.Assets[0].Payload["key"] != "local-sales" {
		t.Fatalf("catalog assets = %#v, want decoded runtime asset", catalog.Assets)
	}
}

func TestAssetCatalogServiceReturnsFalseWithoutActiveOrRuntimeGraph(t *testing.T) {
	catalog, ok, err := NewAssetCatalogService(fakeCatalogRepo{}).ActiveAssetCatalog(context.Background(), "test", "dev")
	if err != nil {
		t.Fatalf("ActiveAssetCatalog() error = %v", err)
	}
	if ok || len(catalog.Assets) != 0 || len(catalog.Edges) != 0 {
		t.Fatalf("catalog = %#v ok=%t, want empty false", catalog, ok)
	}
}

type fakeCatalogRepo struct {
	graph AssetGraph
	ok    bool
	err   error
}

func (r fakeCatalogRepo) Ensure(context.Context, EnsureInput) error {
	return nil
}

func (r fakeCatalogRepo) List(context.Context) ([]Summary, error) {
	return nil, nil
}

func (r fakeCatalogRepo) ByID(context.Context, WorkspaceID) (Summary, error) {
	return Summary{}, nil
}

func (r fakeCatalogRepo) ActiveDeploymentGraph(context.Context, WorkspaceID, string) (AssetGraph, bool, error) {
	return r.graph, r.ok, r.err
}

type fakeRuntimeAssetGraphProvider struct {
	assets []Asset
	edges  []AssetEdge
	ok     bool
}

func (p fakeRuntimeAssetGraphProvider) WorkspaceAssets(string, string) ([]Asset, []AssetEdge, bool) {
	return p.assets, p.edges, p.ok
}

func mustCatalogTestAsset(t *testing.T, workspaceID WorkspaceID, deploymentID DeploymentID, typ AssetType, key string) Asset {
	t.Helper()
	asset, err := NewAsset(workspaceID, deploymentID, typ, key, "", key, "", PayloadSchemaForAssetType(typ), map[string]any{"key": key})
	if err != nil {
		t.Fatalf("NewAsset(): %v", err)
	}
	return asset
}
