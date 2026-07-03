package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type RuntimeProvider interface {
	Active() (runtimehost.Runtime, error)
}

type runtimeProvider = RuntimeProvider

type runtimeMetrics struct {
	provider    runtimeProvider
	dataDir     string
	workspaceID string
}

type dynamicRuntimeMetrics struct {
	defaultID string
	dataDir   string
	factory   func(workspaceID string) RuntimeProvider
	mu        sync.Mutex
	metrics   map[string]QueryMetrics
}

type catalogRuntime interface {
	Catalog() dashboard.Catalog
	DefaultDashboardID() string
	ModelIDForDashboard(dashboardID string) string
	Pages(dashboardID string) []dashboard.Page
}

type workspaceAssetRuntime interface {
	WorkspaceAssets(workspaceID, deploymentID string) ([]workspace.Asset, []workspace.AssetEdge, bool)
}

type reportRuntime interface {
	Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool)
	SemanticModel(modelID string) (*semanticmodel.Model, bool)
	DefaultFilters(dashboardID string) dashboard.Filters
}

type dashboardRuntime interface {
	QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error)
}

type tableRuntime interface {
	NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest
	QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error)
}

type semanticQueryRuntime interface {
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)
	PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)
}

type materializationRuntime interface {
	RefreshMaterializations(ctx context.Context, modelID string) error
}

type agentPolicyProvider interface {
	AgentPolicy() workspace.AgentPolicy
}

func NewRuntimeMetrics(provider runtimeProvider, dataDir, workspaceID string) QueryMetrics {
	return runtimeMetrics{provider: provider, dataDir: dataDir, workspaceID: workspaceID}
}

func NewDynamicRuntimeMetrics(defaultWorkspaceID, dataDir string, factory func(workspaceID string) RuntimeProvider) QueryMetrics {
	return &dynamicRuntimeMetrics{
		defaultID: defaultWorkspaceID,
		dataDir:   dataDir,
		factory:   factory,
		metrics:   map[string]QueryMetrics{},
	}
}

func (m *dynamicRuntimeMetrics) MetricsForWorkspace(workspaceID string) (QueryMetrics, bool) {
	if workspaceID == "" || m.factory == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if metrics := m.metrics[workspaceID]; metrics != nil {
		return metrics, true
	}
	provider := m.factory(workspaceID)
	if provider == nil {
		return nil, false
	}
	metrics := NewRuntimeMetrics(provider, m.dataDir, workspaceID)
	m.metrics[workspaceID] = metrics
	return metrics, true
}

func (m *dynamicRuntimeMetrics) defaultMetrics() QueryMetrics {
	return nil
}

func (m runtimeMetrics) Catalog() dashboard.Catalog {
	runtime, err := m.catalogRuntime()
	if err != nil {
		title := strings.TrimSpace(m.workspaceID)
		if title == "" {
			title = "LibreDash"
		}
		return dashboard.Catalog{
			Workspace: dashboard.CatalogWorkspace{ID: m.workspaceID, Title: title, Description: "No active deployment."},
		}
	}
	return runtime.Catalog()
}

func (m runtimeMetrics) DefaultDashboardID() string {
	runtime, err := m.catalogRuntime()
	if err != nil {
		return ""
	}
	return runtime.DefaultDashboardID()
}

func (m runtimeMetrics) ModelIDForDashboard(dashboardID string) string {
	runtime, err := m.catalogRuntime()
	if err != nil {
		return ""
	}
	return runtime.ModelIDForDashboard(dashboardID)
}

func (m runtimeMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	runtime, err := m.reportRuntime()
	if err != nil {
		return reportdef.Dashboard{}, nil, false
	}
	return runtime.Report(dashboardID)
}

func (m runtimeMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	runtime, err := m.reportRuntime()
	if err != nil {
		return nil, false
	}
	return runtime.SemanticModel(modelID)
}

func (m runtimeMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	runtime, err := m.reportRuntime()
	if err != nil {
		return dashboard.Filters{}.WithDefaults()
	}
	return runtime.DefaultFilters(dashboardID)
}

func (m runtimeMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	runtime, err := m.tableRuntime()
	if err != nil {
		return request.WithDefaults()
	}
	return runtime.NormalizeTableRequest(dashboardID, request)
}

func (m runtimeMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m runtimeMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	runtime, err := m.dashboardRuntime()
	if err != nil {
		return dashboard.EmptyPatch(filters.WithDefaults(), m.dataDir, err), nil
	}
	return runtime.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (m runtimeMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m runtimeMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	runtime, err := m.tableRuntime()
	if err != nil {
		return dashboard.EmptyTable(request.WithDefaults(), err), nil
	}
	return runtime.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func (m runtimeMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	runtime, err := m.semanticQueryRuntime()
	if err != nil {
		return nil, err
	}
	return runtime.QuerySemantic(ctx, modelID, request)
}

func (m runtimeMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	runtime, err := m.semanticQueryRuntime()
	if err != nil {
		return dataquery.Result{}, err
	}
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.workspaceID
	}
	return runtime.ExecuteDataQuery(ctx, request)
}

func (m runtimeMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	runtime, err := m.semanticQueryRuntime()
	if err != nil {
		return nil, err
	}
	return runtime.PreviewSemantic(ctx, modelID, request)
}

func (m runtimeMetrics) ExplainSemanticQuery(modelID string, request reportdef.AggregateQuery) (semanticquery.Plan, error) {
	model, ok := m.SemanticModel(modelID)
	if !ok {
		return semanticquery.Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return semanticquery.NewPlanner(model).Plan(reportdef.SemanticAggregateRequest(request))
}

func (m runtimeMetrics) ExplainSemanticPreview(modelID string, request reportdef.RowQuery) (semanticquery.Plan, error) {
	model, ok := m.SemanticModel(modelID)
	if !ok {
		return semanticquery.Plan{}, fmt.Errorf("unknown semantic model %q", modelID)
	}
	return semanticquery.NewPlanner(model).PlanRows(reportdef.SemanticRowRequest(request))
}

func (m runtimeMetrics) RefreshMaterializations(ctx context.Context, modelID string) error {
	runtime, err := m.materializationRuntime()
	if err != nil {
		return err
	}
	return runtime.RefreshMaterializations(ctx, modelID)
}

func (m runtimeMetrics) RefreshModelTables(ctx context.Context, modelID string, tableNames []string) error {
	runtime, err := m.materializationRuntime()
	if err != nil {
		return err
	}
	port, ok := runtime.(interface {
		RefreshTables(context.Context, string, []string) error
	})
	if !ok {
		return fmt.Errorf("active runtime does not support model table refresh")
	}
	return port.RefreshTables(ctx, modelID, tableNames)
}

func (m runtimeMetrics) DataDir() string {
	return m.dataDir
}

func (m runtimeMetrics) Pages(dashboardID string) []dashboard.Page {
	runtime, err := m.catalogRuntime()
	if err != nil {
		return nil
	}
	return runtime.Pages(dashboardID)
}

func (m runtimeMetrics) WorkspaceAssets(workspaceID, deploymentID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	runtime, err := m.active()
	if err != nil {
		return nil, nil, false
	}
	port, ok := runtime.(workspaceAssetRuntime)
	if !ok {
		return nil, nil, false
	}
	return port.WorkspaceAssets(workspaceID, deploymentID)
}

func (m runtimeMetrics) catalogRuntime() (catalogRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(catalogRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide catalog data")
	}
	return port, nil
}

func (m runtimeMetrics) reportRuntime() (reportRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(reportRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide report data")
	}
	return port, nil
}

func (m runtimeMetrics) dashboardRuntime() (dashboardRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(dashboardRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide dashboard data")
	}
	return port, nil
}

func (m runtimeMetrics) tableRuntime() (tableRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(tableRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide table data")
	}
	return port, nil
}

func (m runtimeMetrics) semanticQueryRuntime() (semanticQueryRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(semanticQueryRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide semantic query data")
	}
	return port, nil
}

func (m runtimeMetrics) materializationRuntime() (materializationRuntime, error) {
	runtime, err := m.active()
	if err != nil {
		return nil, err
	}
	port, ok := runtime.(materializationRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide materialization refresh")
	}
	return port, nil
}

func (m runtimeMetrics) AgentPolicy() workspace.AgentPolicy {
	runtime, err := m.active()
	if err != nil {
		return workspace.DefaultAgentPolicy()
	}
	provider, ok := runtime.(agentPolicyProvider)
	if !ok {
		return workspace.DefaultAgentPolicy()
	}
	return provider.AgentPolicy()
}

func (m runtimeMetrics) active() (runtimehost.Runtime, error) {
	if m.provider == nil {
		return nil, fmt.Errorf("runtime provider is not configured")
	}
	return m.provider.Active()
}
