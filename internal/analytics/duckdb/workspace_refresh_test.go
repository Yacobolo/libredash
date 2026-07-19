package duckdb

import (
	"context"
	"errors"
	"testing"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/runtimehost"
	servingstate "github.com/Yacobolo/libredash/internal/servingstate"
	"github.com/Yacobolo/libredash/internal/workspace"
	"github.com/Yacobolo/libredash/internal/workspace/refresh"
)

func TestWorkspaceRefreshMaterializerResolvesCandidateManagedDataAndReleasesLifetime(t *testing.T) {
	lifetime := &recordingManagedDataLifetime{}
	resolver := &recordingManagedDataResolver{resolution: runtimehost.ManagedDataResolution{
		Roots:    map[string]string{},
		Lifetime: lifetime,
	}}
	materializer := WorkspaceRefreshMaterializer{
		DuckDBDir:       t.TempDir(),
		DuckLakeCatalog: t.TempDir() + "/catalog.sqlite",
		DuckLakeData:    t.TempDir(),
		ManagedData:     resolver,
	}
	_, _ = materializer.Materialize(t.Context(), refresh.MaterializeInput{
		Definition:  &workspace.Definition{Models: map[string]*semanticmodel.Model{}},
		Candidate:   servingstate.State{ID: "candidate-sales", WorkspaceID: "sales", Environment: "dev"},
		Environment: "dev",
	})
	if resolver.servingStateID != "candidate-sales" {
		t.Fatalf("resolved serving state = %q", resolver.servingStateID)
	}
	if !lifetime.released {
		t.Fatal("managed data lifetime was not released after materialization")
	}
}

type recordingManagedDataResolver struct {
	resolution     runtimehost.ManagedDataResolution
	servingStateID servingstate.ID
}

func (r *recordingManagedDataResolver) ResolveManagedData(_ context.Context, id servingstate.ID) (runtimehost.ManagedDataResolution, error) {
	r.servingStateID = id
	return r.resolution, nil
}

type recordingManagedDataLifetime struct {
	released bool
}

func (l *recordingManagedDataLifetime) Release() error {
	if l.released {
		return errors.New("released twice")
	}
	l.released = true
	return nil
}
