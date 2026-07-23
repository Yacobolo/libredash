package compiler

import (
	"testing"

	"github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
)

func TestCompiledVisualizationResultShapes(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		visual report.Visual
		want   visualizationdefinition.ResultShape
	}{
		"scalar":          {report.Visual{Type: "kpi"}, visualizationdefinition.ResultScalar},
		"category":        {report.Visual{Type: "bar"}, visualizationdefinition.ResultCategoryValue},
		"series":          {report.Visual{Type: "line", Query: report.VisualQuery{Series: report.FieldRef{Field: "series"}}}, visualizationdefinition.ResultCategorySeriesValue},
		"multi measure":   {report.Visual{Type: "combo", Query: report.VisualQuery{Measures: []report.FieldRef{{Field: "one"}, {Field: "two"}}}}, visualizationdefinition.ResultCategoryMultiMeasure},
		"waterfall":       {report.Visual{Type: "waterfall"}, visualizationdefinition.ResultCategoryDelta},
		"histogram":       {report.Visual{Type: "histogram"}, visualizationdefinition.ResultHistogramBins},
		"matrix cells":    {report.Visual{Type: "heatmap"}, visualizationdefinition.ResultMatrixCells},
		"hierarchy nodes": {report.Visual{Type: "sunburst"}, visualizationdefinition.ResultHierarchyNodes},
		"graph edges":     {report.Visual{Type: "sankey"}, visualizationdefinition.ResultGraphEdges},
		"ohlc":            {report.Visual{Type: "candlestick"}, visualizationdefinition.ResultOHLC},
		"distribution":    {report.Visual{Type: "boxplot"}, visualizationdefinition.ResultDistribution},
		"geographic":      {report.Visual{Type: "map"}, visualizationdefinition.ResultGeographicFeatures},
		"custom":          {report.Visual{Type: "custom"}, visualizationdefinition.ResultCustomRows},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := compiledVisualResultShape(test.visual)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("result shape = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompiledVisualizationResultShapeFailsClosed(t *testing.T) {
	if _, err := compiledVisualResultShape(report.Visual{Type: "unknown"}); err == nil {
		t.Fatal("unknown visualization result shape passed compilation")
	}
}

func TestCompiledTabularResultShapes(t *testing.T) {
	t.Parallel()
	tests := map[string]visualizationdefinition.ResultShape{
		"table":  visualizationdefinition.ResultDetailWindow,
		"matrix": visualizationdefinition.ResultMatrixWindow,
		"pivot":  visualizationdefinition.ResultPivotWindow,
	}
	for visualType, want := range tests {
		binding := compiledTableBinding("model", visualType, report.TableVisual{Query: report.TableQuery{Table: "orders"}})
		if binding.ResultShape != want {
			t.Fatalf("%s result shape = %q, want %q", visualType, binding.ResultShape, want)
		}
	}
}
