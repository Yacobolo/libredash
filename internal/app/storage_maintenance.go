package app

import (
	"context"
	"fmt"

	storagemaintenance "github.com/Yacobolo/libredash/internal/storage/maintenance"
)

type leasedSnapshotProvider interface {
	LeasedSnapshots() []int64
}

type retentionRepository interface {
	storagemaintenance.ServingStateRepository
}

func (s *Server) reconcileStorageRetention(ctx context.Context, dryRun bool) error {
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
