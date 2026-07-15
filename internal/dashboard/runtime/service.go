package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/internal/workspace"
)

type DataRuntimeConfig struct {
	ModelID string
	Model   *semanticmodel.Model
	DBDir   string
}

type DataRuntimeFactory interface {
	OpenDashboardDataRuntime(ctx context.Context, config DataRuntimeConfig) (DataRuntime, error)
}

type WorkspaceDataRuntimeConfig struct {
	Definition *workspace.Definition
	DBDir      string
}

type WorkspaceDataRuntimeFactory interface {
	OpenDashboardWorkspaceDataRuntimes(ctx context.Context, config WorkspaceDataRuntimeConfig) (map[string]DataRuntime, error)
}

type DataRuntime interface {
	reportdef.DataService
	ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error)
	Refresh(ctx context.Context) error
	Close() error
	LastRefresh() time.Time
}

type DataRuntimeSnapshot interface {
	DuckLakeSnapshotID() int64
}

type setupRequiredError interface {
	SetupRequired() bool
}

type Service struct {
	mu               sync.RWMutex
	runtimes         map[string]*modelRuntime
	catalog          *CatalogService
	reports          *ReportService
	queries          *QueryService
	filters          *FilterService
	visuals          *VisualQueryService
	tables           *TableQueryService
	snapshots        *SnapshotService
	materializations *MaterializationService
}

type modelRuntime struct {
	model   *semanticmodel.Model
	data    DataRuntime
	ready   bool
	missing error
}

func NewFromDefinition(duckDBDir string, factory DataRuntimeFactory, definition *workspace.Definition) (*Service, error) {
	if factory == nil {
		return nil, fmt.Errorf("dashboard data runtime factory is required")
	}
	if definition == nil {
		return nil, fmt.Errorf("workspace definition is required")
	}
	return newFromDefinition(duckDBDir, factory, definition)
}

func newFromDefinition(duckDBDir string, factory DataRuntimeFactory, definition *workspace.Definition) (*Service, error) {
	service := &Service{
		runtimes: map[string]*modelRuntime{},
	}
	service.catalog = NewCatalogService(&service.mu, definition)
	service.reports = &ReportService{
		workspace: definition,
		defaultID: definition.Catalog.Dashboards[0].ID,
	}
	service.filters = &FilterService{}
	service.visuals = &VisualQueryService{filters: service.filters}
	service.tables = &TableQueryService{
		mu:       &service.mu,
		reports:  service.reports,
		runtimes: service.runtimes,
		filters:  service.filters,
	}
	service.snapshots = &SnapshotService{
		mu:       &service.mu,
		reports:  service.reports,
		runtimes: service.runtimes,
		filters:  service.filters,
		visuals:  service.visuals,
	}
	service.queries = &QueryService{
		snapshots: service.snapshots,
		tables:    service.tables,
	}
	service.materializations = &MaterializationService{
		mu:       &service.mu,
		runtimes: service.runtimes,
	}

	for modelID, model := range definition.Models {
		service.runtimes[modelID] = &modelRuntime{model: model}
	}
	if workspaceFactory, ok := factory.(WorkspaceDataRuntimeFactory); ok {
		dataRuntimes, err := workspaceFactory.OpenDashboardWorkspaceDataRuntimes(context.Background(), WorkspaceDataRuntimeConfig{
			Definition: definition,
			DBDir:      duckDBDir,
		})
		if err != nil {
			if setupRequired(err) {
				for _, runtime := range service.runtimes {
					runtime.missing = err
				}
				return service, nil
			}
			return nil, err
		}
		for modelID, runtime := range service.runtimes {
			dataRuntime, ok := dataRuntimes[modelID]
			if !ok {
				return nil, fmt.Errorf("workspace data runtime missing semantic model %q", modelID)
			}
			runtime.data = newGovernedDataRuntime(definition.Catalog.Workspace.ID, modelID, dataRuntime)
			runtime.ready = true
		}
		for modelID := range dataRuntimes {
			if _, ok := service.runtimes[modelID]; !ok {
				return nil, fmt.Errorf("workspace data runtime returned unknown semantic model %q", modelID)
			}
		}
		return service, nil
	}

	for modelID, model := range definition.Models {
		runtime := service.runtimes[modelID]
		dataRuntime, err := factory.OpenDashboardDataRuntime(context.Background(), DataRuntimeConfig{
			ModelID: modelID,
			Model:   model,
			DBDir:   duckDBDir,
		})
		if err != nil {
			if setupRequired(err) {
				runtime.missing = err
				continue
			}
			return nil, err
		}
		runtime.data = newGovernedDataRuntime(definition.Catalog.Workspace.ID, modelID, dataRuntime)
		runtime.ready = true
	}

	return service, nil
}

func (m *Service) Close() error {
	var closeErr error
	for _, runtime := range m.runtimes {
		if runtime.data == nil {
			continue
		}
		if err := runtime.data.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (m *Service) DuckLakeSnapshotID() int64 {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var snapshotID int64
	for _, runtime := range m.runtimes {
		if runtime == nil || runtime.data == nil {
			continue
		}
		snapshot, ok := runtime.data.(DataRuntimeSnapshot)
		if !ok {
			continue
		}
		current := snapshot.DuckLakeSnapshotID()
		if current == 0 {
			continue
		}
		if snapshotID == 0 {
			snapshotID = current
			continue
		}
		if snapshotID != current {
			return 0
		}
	}
	return snapshotID
}

func setupRequired(err error) bool {
	var setup setupRequiredError
	return errors.As(err, &setup) && setup.SetupRequired()
}
