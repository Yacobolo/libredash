package report

import (
	"slices"
	"strings"
)

var supportedVisualShapes = map[string]struct{}{
	"category_value": {}, "category_series_value": {}, "category_multi_measure": {}, "category_delta": {},
	"single_value": {}, "matrix": {}, "graph": {}, "geo": {}, "ohlc": {}, "distribution": {},
	"binned_measure": {}, "hierarchy": {},
}

func SupportedVisualShapes() []string {
	shapes := make([]string, 0, len(supportedVisualShapes))
	for shape := range supportedVisualShapes {
		shapes = append(shapes, shape)
	}
	slices.Sort(shapes)
	return shapes
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func titleFromIdentifier(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func supportsVisualKind(kind string) bool {
	return kind == "chart" || kind == "kpi"
}

func supportsVisualShape(shape string) bool {
	_, ok := supportedVisualShapes[shape]
	return ok
}

func supportsRenderer(renderer string) bool {
	return renderer == "echarts" || renderer == "html"
}

func rendererSupportsType(renderer, chartType string) bool {
	if renderer == "html" {
		return chartType == "kpi" || chartType == ""
	}
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst":
		return true
	default:
		return false
	}
}

func supportsSeries(shape string) bool {
	return shape == "category_series_value"
}

func rendererSupportsShapeType(renderer, shape, chartType string) bool {
	if renderer == "html" {
		return shape == "single_value" && (chartType == "kpi" || chartType == "")
	}
	if renderer != "echarts" {
		return false
	}
	switch shape {
	case "category_value":
		switch chartType {
		case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "radar":
			return true
		}
	case "category_series_value":
		return rendererTypeSupportsSeries(renderer, chartType)
	case "category_multi_measure":
		return chartType == "combo"
	case "category_delta":
		return chartType == "waterfall"
	case "single_value":
		return chartType == "gauge"
	case "matrix":
		return chartType == "heatmap"
	case "graph":
		return chartType == "sankey" || chartType == "graph"
	case "geo":
		return chartType == "map"
	case "ohlc":
		return chartType == "candlestick"
	case "distribution":
		return chartType == "boxplot"
	case "binned_measure":
		return chartType == "histogram"
	case "hierarchy":
		return chartType == "tree" || chartType == "sunburst"
	}
	return false
}

func rendererTypeSupportsSeries(renderer, chartType string) bool {
	if renderer != "echarts" {
		return false
	}
	switch chartType {
	case "line", "area", "bar", "column", "scatter":
		return true
	default:
		return false
	}
}
