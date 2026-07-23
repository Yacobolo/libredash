package tools

import (
	"context"
	"strings"
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

func TestAgentVisualInputRejectsLegacyAndUnknownProperties(t *testing.T) {
	for _, property := range []string{"shape", "options", "rendererOptions", "unexpected"} {
		t.Run(property, func(t *testing.T) {
			_, err := decodeAgentVisualInput([]byte(`{"type":"histogram","model":"sales","dataset":"orders","` + property + `":{}}`))
			if err == nil || !strings.Contains(err.Error(), property) {
				t.Fatalf("decode error = %v, want closed-contract rejection for %q", err, property)
			}
		})
	}
	for _, property := range []string{`"shape"`, `"options"`, `"rendererOptions"`} {
		if strings.Contains(agentVisualToolSchema, property) {
			t.Fatalf("agent schema still exposes legacy property %s", property)
		}
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
		Measures: []agentVisualFieldRef{{Field: "revenue"}}, Presentation: agentVisualPresentation{HistogramBins: 12},
	}
	data, err := provider.agentChartData(context.Background(), "sales", input, agentVisualShape(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0]["binStart"] != float64(10) || data[0]["binEnd"] != float64(20) || data[0]["value"] != 4 {
		t.Fatalf("histogram data = %#v", data)
	}
}
