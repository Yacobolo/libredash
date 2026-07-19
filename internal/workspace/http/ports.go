package http

import (
	"context"
	nethttp "net/http"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/ui"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type Metrics interface {
	Catalog() dashboard.Catalog
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	Pages(dashboardID string) []dashboard.Page
}

type AssetCatalogReader interface {
	ActiveAssetCatalog(ctx context.Context, workspaceID workspace.WorkspaceID, environment string) (workspace.AssetCatalog, bool, error)
}

type RefreshStateProvider interface {
	AssetRefreshState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView) (ui.AssetRefreshState, error)
	AssetVersionsState(ctx context.Context, workspaceID, environment string, asset workspace.AssetView, section string) (ui.AssetVersionsState, error)
}

type AssetRefreshRunner interface {
	RefreshAsset(ctx context.Context, input AssetRefreshInput) error
}

type AssetRefreshInput struct {
	Request     *nethttp.Request
	WorkspaceID string
	Asset       workspace.AssetView
	Assets      []workspace.AssetView
	Edges       []workspace.AssetEdgeView
}
