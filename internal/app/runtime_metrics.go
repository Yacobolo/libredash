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
	Active(ctx context.Context) (runtimehost.Runtime, error)
}

type runtimeProvider = RuntimeProvider

type runtimeLeaseProvider interface {
	Acquire(ctx context.Context) (runtimehost.Lease, error)
}

type runtimeMetrics struct {
	provider    runtimeProvider
	workspaceID string
}

type dashboardRefreshRuntimeKey struct{}

type dashboardRefreshRuntime struct {
	workspaceID string
	runtime     runtimehost.Runtime
}

type dynamicRuntimeMetrics struct {
	defaultID string
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
	WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool)
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

type agentPolicyProvider interface {
	AgentPolicy() workspace.AgentPolicy
}

func NewRuntimeMetrics(provider runtimeProvider, workspaceID string) QueryMetrics {
	return runtimeMetrics{provider: provider, workspaceID: workspaceID}
}

func NewDynamicRuntimeMetrics(defaultWorkspaceID string, factory func(workspaceID string) RuntimeProvider) QueryMetrics {
	return &dynamicRuntimeMetrics{
		defaultID: defaultWorkspaceID,
		factory:   factory,
		metrics:   map[string]QueryMetrics{},
	}
}

func (m *dynamicRuntimeMetrics) RuntimeReady(ctx context.Context, workspaceID string) error {
	metrics, ok := m.MetricsForWorkspace(workspaceID)
	if !ok || metrics == nil {
		return fmt.Errorf("runtime for workspace %q is not configured", workspaceID)
	}
	if readiness, ok := metrics.(workspaceRuntimeReadiness); ok {
		return readiness.RuntimeReady(ctx, workspaceID)
	}
	return metricsMetadataReady(metrics, workspaceID)
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
	metrics := NewRuntimeMetrics(provider, workspaceID)
	m.metrics[workspaceID] = metrics
	return metrics, true
}

func (m *dynamicRuntimeMetrics) defaultMetrics() QueryMetrics {
	return nil
}

func (m runtimeMetrics) Catalog() dashboard.Catalog {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		title := strings.TrimSpace(m.workspaceID)
		if title == "" {
			title = "LibreDash"
		}
		return dashboard.Catalog{
			Workspace: dashboard.CatalogWorkspace{ID: m.workspaceID, Title: title, Description: "No active serving state."},
		}
	}
	defer release()
	port, ok := runtime.(catalogRuntime)
	if !ok {
		return dashboard.Catalog{}
	}
	return port.Catalog()
}

func (m runtimeMetrics) DefaultDashboardID() string {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return ""
	}
	defer release()
	port, ok := runtime.(catalogRuntime)
	if !ok {
		return ""
	}
	return port.DefaultDashboardID()
}

func (m runtimeMetrics) ModelIDForDashboard(dashboardID string) string {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return ""
	}
	defer release()
	port, ok := runtime.(catalogRuntime)
	if !ok {
		return ""
	}
	return port.ModelIDForDashboard(dashboardID)
}

func (m runtimeMetrics) Report(dashboardID string) (reportdef.Dashboard, *semanticmodel.Model, bool) {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return reportdef.Dashboard{}, nil, false
	}
	defer release()
	port, ok := runtime.(reportRuntime)
	if !ok {
		return reportdef.Dashboard{}, nil, false
	}
	return port.Report(dashboardID)
}

func (m runtimeMetrics) SemanticModel(modelID string) (*semanticmodel.Model, bool) {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return nil, false
	}
	defer release()
	port, ok := runtime.(reportRuntime)
	if !ok {
		return nil, false
	}
	return port.SemanticModel(modelID)
}

func (m runtimeMetrics) DefaultFilters(dashboardID string) dashboard.Filters {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return dashboard.Filters{}.WithDefaults()
	}
	defer release()
	port, ok := runtime.(reportRuntime)
	if !ok {
		return dashboard.Filters{}.WithDefaults()
	}
	return port.DefaultFilters(dashboardID)
}

func (m runtimeMetrics) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return request.WithDefaults()
	}
	defer release()
	port, ok := runtime.(tableRuntime)
	if !ok {
		return request.WithDefaults()
	}
	return port.NormalizeTableRequest(dashboardID, request)
}

func (m runtimeMetrics) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	return m.QueryDashboardPage(ctx, dashboardID, "", filters)
}

func (m runtimeMetrics) QueryDashboardPage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters) (dashboard.Patch, error) {
	runtime, release, err := m.active(ctx)
	if err != nil {
		return dashboard.EmptyPatch(filters.WithDefaults(), err), nil
	}
	defer release()
	port, ok := runtime.(dashboardRuntime)
	if !ok {
		err := fmt.Errorf("active runtime does not provide dashboard data")
		return dashboard.EmptyPatch(filters.WithDefaults(), err), nil
	}
	return port.QueryDashboardPage(ctx, dashboardID, pageID, filters)
}

func (m runtimeMetrics) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	return m.QueryTablePage(ctx, dashboardID, "", filters, request)
}

func (m runtimeMetrics) QueryTablePage(ctx context.Context, dashboardID, pageID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	runtime, release, err := m.activeForDashboardRefresh(ctx)
	if err != nil {
		return dashboard.EmptyTable(request.WithDefaults(), err), nil
	}
	defer release()
	port, ok := runtime.(tableRuntime)
	if !ok {
		return dashboard.EmptyTable(request.WithDefaults(), fmt.Errorf("active runtime does not provide table data")), nil
	}
	return port.QueryTablePage(ctx, dashboardID, pageID, filters, request)
}

func (m runtimeMetrics) WithDashboardRefreshLease(ctx context.Context, run func(context.Context) error) error {
	if run == nil {
		return fmt.Errorf("dashboard refresh lease callback is required")
	}
	if pinned, ok := ctx.Value(dashboardRefreshRuntimeKey{}).(dashboardRefreshRuntime); ok && pinned.workspaceID == m.workspaceID && pinned.runtime != nil {
		return run(ctx)
	}
	runtime, release, err := m.active(ctx)
	if err != nil {
		return err
	}
	defer release()
	ctx = context.WithValue(ctx, dashboardRefreshRuntimeKey{}, dashboardRefreshRuntime{workspaceID: m.workspaceID, runtime: runtime})
	return run(ctx)
}

func (m runtimeMetrics) activeForDashboardRefresh(ctx context.Context) (runtimehost.Runtime, func(), error) {
	if pinned, ok := ctx.Value(dashboardRefreshRuntimeKey{}).(dashboardRefreshRuntime); ok && pinned.workspaceID == m.workspaceID && pinned.runtime != nil {
		return pinned.runtime, func() {}, nil
	}
	return m.active(ctx)
}

func (m runtimeMetrics) QuerySemantic(ctx context.Context, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error) {
	runtime, release, err := m.active(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	port, ok := runtime.(semanticQueryRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide semantic query data")
	}
	return port.QuerySemantic(ctx, modelID, request)
}

func (m runtimeMetrics) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	runtime, release, err := m.active(ctx)
	if err != nil {
		return dataquery.Result{}, err
	}
	defer release()
	port, ok := runtime.(semanticQueryRuntime)
	if !ok {
		return dataquery.Result{}, fmt.Errorf("active runtime does not provide semantic query data")
	}
	if request.WorkspaceID == "" {
		request.WorkspaceID = m.workspaceID
	}
	return port.ExecuteDataQuery(ctx, request)
}

func (m runtimeMetrics) PreviewSemantic(ctx context.Context, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error) {
	runtime, release, err := m.active(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	port, ok := runtime.(semanticQueryRuntime)
	if !ok {
		return nil, fmt.Errorf("active runtime does not provide semantic query data")
	}
	return port.PreviewSemantic(ctx, modelID, request)
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

func (m runtimeMetrics) Pages(dashboardID string) []dashboard.Page {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return nil
	}
	defer release()
	port, ok := runtime.(catalogRuntime)
	if !ok {
		return nil
	}
	return port.Pages(dashboardID)
}

func (m runtimeMetrics) WorkspaceAssets(workspaceID, servingStateID string) ([]workspace.Asset, []workspace.AssetEdge, bool) {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return nil, nil, false
	}
	defer release()
	port, ok := runtime.(workspaceAssetRuntime)
	if !ok {
		return nil, nil, false
	}
	return port.WorkspaceAssets(workspaceID, servingStateID)
}

func (m runtimeMetrics) AgentPolicy() workspace.AgentPolicy {
	runtime, release, err := m.active(context.Background())
	if err != nil {
		return workspace.DefaultAgentPolicy()
	}
	defer release()
	provider, ok := runtime.(agentPolicyProvider)
	if !ok {
		return workspace.DefaultAgentPolicy()
	}
	return provider.AgentPolicy()
}

func (m runtimeMetrics) RuntimeReady(ctx context.Context, workspaceID string) error {
	activeRuntime, release, err := m.active(ctx)
	if err != nil {
		return err
	}
	defer release()
	catalogPort, ok := activeRuntime.(catalogRuntime)
	if !ok {
		return fmt.Errorf("active runtime does not provide catalog metadata")
	}
	catalog := catalogPort.Catalog()
	if workspaceID != "" && catalog.Workspace.ID != "" && catalog.Workspace.ID != workspaceID {
		return fmt.Errorf("catalog workspace = %q, want %q", catalog.Workspace.ID, workspaceID)
	}
	if len(catalog.Models) == 0 && len(catalog.Dashboards) == 0 {
		return fmt.Errorf("runtime catalog is empty")
	}
	if len(catalog.Dashboards) == 0 {
		return nil
	}
	defaultDashboardID := catalogPort.DefaultDashboardID()
	if defaultDashboardID == "" {
		return fmt.Errorf("default dashboard is not configured")
	}
	reportPort, ok := activeRuntime.(reportRuntime)
	if !ok {
		return fmt.Errorf("active runtime does not provide report metadata")
	}
	report, model, ok := reportPort.Report(defaultDashboardID)
	return reportMetadataReady(catalogPort, defaultDashboardID, report, model, ok)
}

func (m runtimeMetrics) active(ctx context.Context) (runtimehost.Runtime, func(), error) {
	if m.provider == nil {
		return nil, func() {}, fmt.Errorf("runtime provider is not configured")
	}
	if provider, ok := m.provider.(runtimeLeaseProvider); ok {
		lease, err := provider.Acquire(ctx)
		if err != nil {
			return nil, func() {}, err
		}
		return lease.Runtime(), lease.Release, nil
	}
	runtime, err := m.provider.Active(ctx)
	return runtime, func() {}, err
}
