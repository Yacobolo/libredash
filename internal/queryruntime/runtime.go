package queryruntime

import (
	"context"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type Metrics interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	ModelIDForDashboard(dashboardID string) string
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
	DefaultFilters(dashboardID string) dashboard.Filters
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
	QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)
	PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)
	RefreshMaterializations(ctx context.Context, modelID string) error
	DataDir() string
	Pages(dashboardID string) []dashboard.Page
}

type WorkspaceMetrics interface {
	MetricsForWorkspace(workspaceID string) (Metrics, bool)
}
