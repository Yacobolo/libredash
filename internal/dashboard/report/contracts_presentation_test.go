package report

import (
	"strings"
	"testing"
)

func TestValidateVisualPresentationRejectsUnsupportedRendererValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		visualType   string
		presentation VisualPresentation
		want         string
	}{
		{name: "funnel align", visualType: "funnel", presentation: VisualPresentation{Align: "middle"}, want: "presentation.align"},
		{name: "funnel sort", visualType: "funnel", presentation: VisualPresentation{Sort: "random"}, want: "presentation.sort"},
		{name: "hierarchy layout", visualType: "graph", presentation: VisualPresentation{Layout: "force"}, want: "presentation.layout"},
		{name: "graph focus", visualType: "graph", presentation: VisualPresentation{Focus: "series"}, want: "presentation.focus"},
		{name: "negative depth", visualType: "tree", presentation: VisualPresentation{InitialDepth: -1}, want: "presentation.initial_depth"},
		{name: "negative node gap", visualType: "sankey", presentation: VisualPresentation{NodeGap: -1}, want: "presentation.node_gap"},
		{name: "curveness range", visualType: "graph", presentation: VisualPresentation{Curveness: 1.1}, want: "presentation.curveness"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateVisualPresentation("visual", Visual{Type: test.visualType, Presentation: test.presentation})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateVisualPresentation() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateVisualPresentationAcceptsEChartsTypedValues(t *testing.T) {
	t.Parallel()

	valid := []Visual{
		{Type: "funnel", Presentation: VisualPresentation{Align: "left", Sort: "ascending"}},
		{Type: "graph", Presentation: VisualPresentation{Layout: "circular", Focus: "adjacency", Curveness: 0.4}},
		{Type: "sankey", Presentation: VisualPresentation{NodeGap: 18, Curveness: 0.3}},
		{Type: "tree", Presentation: VisualPresentation{Layout: "standard", InitialDepth: 2}},
	}
	for _, visual := range valid {
		if err := validateVisualPresentation("visual", visual); err != nil {
			t.Fatalf("validateVisualPresentation(%s): %v", visual.Type, err)
		}
	}
}
