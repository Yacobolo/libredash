package tools

import (
	"context"
	"testing"

	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
)

func TestAgentVisualShapeUsesVisualTypeDefaults(t *testing.T) {
	tests := map[string]string{
		"histogram":   "binned_measure",
		"candlestick": "ohlc",
		"boxplot":     "distribution",
		"heatmap":     "matrix",
		"sankey":      "graph",
		"map":         "geo",
		"sunburst":    "hierarchy",
		"kpi":         "single_value",
	}
	for visualType, want := range tests {
		t.Run(visualType, func(t *testing.T) {
			if got := agentVisualShape(agentVisualInput{Type: visualType}); got != want {
				t.Fatalf("shape = %q, want %q", got, want)
			}
		})
	}
}

func TestAgentVisualContractRejectsIncompatibleExplicitShape(t *testing.T) {
	input := agentVisualInput{
		Type: "histogram", Shape: "single_value", Dataset: "orders",
		Measures: []agentVisualFieldRef{{Field: "revenue"}},
	}
	if err := validateAgentChartContract(input); err == nil {
		t.Fatal("incompatible histogram shape was accepted")
	}
}

func TestAgentHistogramProducesBinnedPayload(t *testing.T) {
	provider := VisualProvider{
		Histogram: func(context.Context, string, string, reportdef.RawValueQuery, int) ([]reportdef.HistogramBin, error) {
			return []reportdef.HistogramBin{{Bucket: 0, Count: 4, Start: 10, End: 20}}, nil
		},
	}
	input := agentVisualInput{
		Type: "histogram", Dataset: "orders", Model: "sales",
		Measures: []agentVisualFieldRef{{Field: "revenue"}}, Options: map[string]any{"bin_count": 12},
	}
	data, err := provider.agentChartData(context.Background(), "sales", input, agentVisualShape(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0]["binStart"] != float64(10) || data[0]["binEnd"] != float64(20) || data[0]["value"] != 4 {
		t.Fatalf("histogram data = %#v", data)
	}
}
