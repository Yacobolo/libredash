package compiler

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard"
	dashboardfilter "github.com/Yacobolo/leapview/internal/dashboard/filter"
)

func TestProjectDashboardPagesPreserveCanonicalComponentKinds(t *testing.T) {
	pages := projectDashboardPages([]projectDashboardPage{{
		ID: "overview",
		Components: []dashboard.PageVisual{
			{ID: "state", Kind: "slicer", Binding: dashboardfilter.BindingRef{Scope: dashboardfilter.ScopePage, ID: "state"}},
			{ID: "orders", Kind: "visual", Visual: "orders"},
			{ID: "rows", Kind: "visual", Visual: "rows"},
		},
	}})

	got := pages[0].Visuals
	if got[0].Kind != "slicer" || got[1].Kind != "visual" || got[2].Kind != "visual" {
		t.Fatalf("component kinds = %q, %q, %q; want slicer, visual, visual", got[0].Kind, got[1].Kind, got[2].Kind)
	}
}
