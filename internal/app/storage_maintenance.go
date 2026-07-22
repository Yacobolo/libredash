package app

import (
	"context"
	"fmt"

	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	storagemaintenance "github.com/Yacobolo/leapview/internal/storage/maintenance"
	"github.com/Yacobolo/leapview/internal/workload"
)

type leasedSnapshotProvider interface {
	LeasedSnapshots() []int64
}

type retentionRepository interface {
	storagemaintenance.ServingStateRepository
}

func (s *Server) reconcileStorageRetention(ctx context.Context, dryRun bool) error {
	if _, _, admitted := workload.Current(ctx); !admitted {
		lease, err := s.workloadController().Acquire(ctx, workload.Request{Class: workload.Maintenance, Operation: "storage.retention"})
		if err != nil {
			return nil
		}
		defer lease.Release()
		ctx = lease.Context()
	}
	repo, ok := s.servingStateRepo.(retentionRepository)
	if !ok || repo == nil {
		return nil
	}
	rootDir := ""
	catalogPath := s.duckLakeCatalogPath
	dataPath := s.duckLakeDataPath
	if catalogPath == "" || dataPath == "" {
		return nil
	}
	protected := []int64(nil)
	if provider, ok := s.reloader.(leasedSnapshotProvider); ok {
		protected = provider.LeasedSnapshots()
	}
	_, err := storagemaintenance.Run(ctx, repo, storagemaintenance.Options{
		DuckDBEnvironment:            s.duckDBEnvironment,
		Environment:                  servingstate.Environment(s.defaultEnvironment),
		RootDir:                      rootDir,
		CatalogPath:                  catalogPath,
		DataPath:                     dataPath,
		AdditionalProtectedSnapshots: protected,
		DryRun:                       dryRun,
	})
	if err != nil {
		return fmt.Errorf("storage retention: %w", err)
	}
	return nil
}
