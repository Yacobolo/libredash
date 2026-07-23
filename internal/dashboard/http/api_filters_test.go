package http

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
)

func TestDashboardFiltersProvidedIncludesSpatialSelections(t *testing.T) {
	filters := dashboard.Filters{
		SpatialSelections: []dashboard.SpatialInteractionSelection{{VisualID: "customer-map"}},
	}
	if !dashboardFiltersProvided(filters) {
		t.Fatal("spatial-only dashboard filters must not be replaced by defaults")
	}
}

func TestDashboardFiltersProvidedRejectsAnEmptyFilterSet(t *testing.T) {
	if dashboardFiltersProvided(dashboard.Filters{}) {
		t.Fatal("empty dashboard filters should use defaults")
	}
}
