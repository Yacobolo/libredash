package duckdb

import (
	"context"
	"errors"
	"testing"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/runtimehost"
	servingstate "github.com/Yacobolo/leapview/internal/servingstate"
	"github.com/Yacobolo/leapview/internal/workspace"
	"github.com/Yacobolo/leapview/internal/workspace/refresh"
)

func TestWorkspaceRefreshMaterializerResolvesCandidateManagedDataAndReleasesLifetime(t *testing.T) {
	lifetime := &recordingManagedDataLifetime{}
	resolver := &recordingManagedDataResolver{resolution: runtimehost.ManagedDataResolution{
		Roots:    map[string]string{},
		Lifetime: lifetime,
	}}
	materializer := WorkspaceRefreshMaterializer{
		ManagedData: resolver,
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

func TestApplyDiscoveredSourceSchemasPreservesAuthoredMetadata(t *testing.T) {
	nullable := true
	refreshed := &semanticmodel.Model{Sources: map[string]semanticmodel.Source{
		"orders": {Schema: semanticmodel.TableSchema{Columns: []semanticmodel.ColumnSchema{{
			Name: "order_id", Ordinal: 1, PhysicalType: "VARCHAR", Nullable: &nullable,
		}}}},
	}}
	authored := &semanticmodel.Model{Sources: map[string]semanticmodel.Source{
		"orders": {Description: "Authored source", Fields: map[string]semanticmodel.SourceField{
			"order_id": {Description: "Authored field"},
		}},
	}}

	applyDiscoveredSourceSchemas(refreshed, map[string]*semanticmodel.Model{"commerce": authored})

	source := authored.Sources["orders"]
	if source.Description != "Authored source" || source.Fields["order_id"].Description != "Authored field" {
		t.Fatalf("authored metadata was replaced: %#v", source)
	}
	if len(source.Schema.Columns) != 1 || source.Schema.Columns[0].Name != "order_id" || source.Schema.Columns[0].PhysicalType != "VARCHAR" {
		t.Fatalf("discovered schema was not propagated: %#v", source.Schema)
	}
	refreshedColumn := refreshed.Sources["orders"]
	*refreshedColumn.Schema.Columns[0].Nullable = false
	if source.Schema.Columns[0].Nullable == nil || !*source.Schema.Columns[0].Nullable {
		t.Fatal("propagated source schema aliases refresh-owned metadata")
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
