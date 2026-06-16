package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/data"
	"github.com/Yacobolo/libredash/internal/deploy"
	"github.com/Yacobolo/libredash/internal/platform"
	platformdb "github.com/Yacobolo/libredash/internal/platform/db"
	"github.com/Yacobolo/libredash/internal/semantic"
)

type Manager struct {
	mu          sync.RWMutex
	store       *platform.Store
	workspaceID string
	dataDir     string
	duckDBDir   string
	runtimeDir  string

	activeDeployment string
	activeDigest     string
	current          *data.DuckDBMetrics
}

type Prepared struct {
	deploymentID string
	digest       string
	metrics      *data.DuckDBMetrics
	noChange     bool
}

func (p *Prepared) Close() error {
	if p == nil || p.metrics == nil {
		return nil
	}
	return p.metrics.Close()
}

func NewManager(store *platform.Store, workspaceID, dataDir, duckDBDir, runtimeDir string) *Manager {
	return &Manager{
		store:       store,
		workspaceID: workspaceID,
		dataDir:     dataDir,
		duckDBDir:   duckDBDir,
		runtimeDir:  runtimeDir,
	}
}

func (m *Manager) Reload(ctx context.Context) error {
	deployment, artifact, err := m.store.ActiveArtifact(ctx, m.workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	prepared, err := m.prepare(ctx, deployment, artifact)
	if err != nil {
		return err
	}
	return m.CommitPrepared(prepared)
}

func (m *Manager) PrepareDeployment(ctx context.Context, deploymentID string) (*Prepared, error) {
	deployment, err := m.store.Queries().GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if deployment.WorkspaceID != m.workspaceID {
		return nil, fmt.Errorf("deployment %s is not in workspace %s", deploymentID, m.workspaceID)
	}
	artifact, err := m.store.Queries().GetArtifactByDeployment(ctx, deployment.ID)
	if err != nil {
		return nil, err
	}
	return m.prepare(ctx, deployment, artifact)
}

func (m *Manager) prepare(ctx context.Context, deployment platformdb.Deployment, artifact platformdb.DeploymentArtifact) (*Prepared, error) {
	m.mu.RLock()
	if m.current != nil && m.activeDeployment == deployment.ID && m.activeDigest == artifact.Digest {
		m.mu.RUnlock()
		return &Prepared{deploymentID: deployment.ID, digest: artifact.Digest, noChange: true}, nil
	}
	m.mu.RUnlock()

	targetDir := filepath.Join(m.runtimeDir, deployment.ID+"-"+shortDigest(artifact.Digest))
	if err := os.RemoveAll(targetDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := deploy.ExtractArtifact(artifact.Path, targetDir); err != nil {
		return nil, err
	}
	duckDir := filepath.Join(m.duckDBDir, deployment.ID)
	metrics, err := data.NewDuckDBMetricsFromCatalog(m.dataDir, filepath.Join(targetDir, deploy.CatalogFile), duckDir)
	if err != nil {
		return nil, err
	}
	return &Prepared{deploymentID: deployment.ID, digest: artifact.Digest, metrics: metrics}, nil
}

func (m *Manager) CommitPrepared(prepared *Prepared) error {
	if prepared == nil {
		return fmt.Errorf("prepared runtime is nil")
	}
	if prepared.noChange {
		return nil
	}

	m.mu.Lock()
	old := m.current
	m.current = prepared.metrics
	m.activeDeployment = prepared.deploymentID
	m.activeDigest = prepared.digest
	prepared.metrics = nil
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func shortDigest(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	return m.current.Close()
}

func (m *Manager) metrics() (*data.DuckDBMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return nil, fmt.Errorf("no active LibreDash deployment")
	}
	return m.current, nil
}

func (m *Manager) Catalog() dashboard.Catalog {
	metrics, err := m.metrics()
	if err != nil {
		return dashboard.Catalog{
			Workspace: dashboard.CatalogWorkspace{ID: m.workspaceID, Title: "LibreDash Workspace", Description: "No active deployment."},
		}
	}
	return metrics.Catalog()
}

func (m *Manager) DefaultDashboardID() string {
	metrics, err := m.metrics()
	if err != nil {
		return ""
	}
	return metrics.DefaultDashboardID()
}

func (m *Manager) ModelIDForDashboard(dashboardID string) string {
	metrics, err := m.metrics()
	if err != nil {
		return ""
	}
	return metrics.ModelIDForDashboard(dashboardID)
}

func (m *Manager) Report(dashboardID string) (semantic.Dashboard, *semantic.Model, bool) {
	metrics, err := m.metrics()
	if err != nil {
		return semantic.Dashboard{}, nil, false
	}
	return metrics.Report(dashboardID)
}

func (m *Manager) DefaultFilters(dashboardID string) dashboard.Filters {
	metrics, err := m.metrics()
	if err != nil {
		return dashboard.Filters{}.WithDefaults()
	}
	return metrics.DefaultFilters(dashboardID)
}

func (m *Manager) NormalizeTableRequest(dashboardID string, request dashboard.TableRequest) dashboard.TableRequest {
	metrics, err := m.metrics()
	if err != nil {
		return request.WithDefaults()
	}
	return metrics.NormalizeTableRequest(dashboardID, request)
}

func (m *Manager) QueryDashboard(ctx context.Context, dashboardID string, filters dashboard.Filters) (dashboard.Patch, error) {
	metrics, err := m.metrics()
	if err != nil {
		return dashboard.EmptyPatch(filters.WithDefaults(), m.dataDir, err), nil
	}
	return metrics.QueryDashboard(ctx, dashboardID, filters)
}

func (m *Manager) QueryTable(ctx context.Context, dashboardID string, filters dashboard.Filters, request dashboard.TableRequest) (dashboard.Table, error) {
	metrics, err := m.metrics()
	if err != nil {
		return dashboard.EmptyTable(request.WithDefaults(), err), nil
	}
	return metrics.QueryTable(ctx, dashboardID, filters, request)
}

func (m *Manager) RefreshCache(ctx context.Context, modelID string) error {
	metrics, err := m.metrics()
	if err != nil {
		return err
	}
	return metrics.RefreshCache(ctx, modelID)
}

func (m *Manager) DataDir() string {
	return m.dataDir
}

func (m *Manager) Pages(dashboardID string) []dashboard.Page {
	metrics, err := m.metrics()
	if err != nil {
		return nil
	}
	return metrics.Pages(dashboardID)
}

func (m *Manager) ModelGraph(modelID string) (dashboard.ModelGraph, bool) {
	metrics, err := m.metrics()
	if err != nil {
		return dashboard.ModelGraph{}, false
	}
	return metrics.ModelGraph(modelID)
}
