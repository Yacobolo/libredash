package data

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Yacobolo/libredash/internal/dashboard"
)

func pointSelected(points []dashboard.Datum, label string) bool {
	for _, point := range points {
		if datumString(point, "label") == label {
			selected, _ := point["selected"].(bool)
			return selected
		}
	}
	return false
}

func overviewVisualKeys() []string {
	return []string{"aov_kpi", "categories", "delivery", "orders", "revenue", "revenue_kpi", "review_kpi", "total_orders"}
}

func chartShowcaseMatrix() map[string][]string {
	return map[string][]string{
		"line":        {"revenue_line", "revenue_line_status", "revenue_line_step"},
		"area":        {"revenue", "revenue_area_status", "revenue_area_smooth"},
		"bar":         {"categories", "delivery", "categories_by_status_bar"},
		"column":      {"orders_by_month_column", "orders_by_month_status", "orders_by_month_status_grouped"},
		"pie":         {"status_pie", "status_pie_rose", "category_pie_inside"},
		"donut":       {"orders", "category_donut", "orders_donut_center"},
		"scatter":     {"delivery_scatter", "delivery_scatter_status", "delivery_scatter_labeled"},
		"funnel":      {"status_funnel", "delivery_funnel", "status_funnel_left"},
		"treemap":     {"category_treemap", "state_treemap", "category_treemap_roam"},
		"gauge":       {"total_orders_gauge", "review_gauge", "review_gauge_thresholds"},
		"heatmap":     {"state_status_heatmap", "category_status_heatmap", "category_status_heatmap_labels"},
		"sankey":      {"status_delivery_flow", "category_status_flow", "category_status_flow_spacious"},
		"graph":       {"status_delivery_graph", "category_status_graph", "category_status_graph_circular"},
		"map":         {"state_order_map", "state_revenue_map", "state_revenue_map_labeled"},
		"candlestick": {"delivery_candlestick", "revenue_candlestick"},
		"boxplot":     {"delivery_distribution", "review_distribution", "revenue_distribution"},
		"combo":       {"revenue_orders_combo", "review_delivery_combo", "revenue_orders_dual_axis_combo"},
		"waterfall":   {"revenue_waterfall", "orders_waterfall", "revenue_waterfall_labeled"},
		"histogram":   {"delivery_histogram", "revenue_histogram", "review_histogram"},
		"radar":       {"status_radar", "delivery_radar", "state_radar"},
		"tree":        {"state_status_tree", "category_status_tree", "category_state_status_tree"},
		"sunburst":    {"category_status_sunburst", "state_status_sunburst", "category_state_status_sunburst"},
	}
}

func assertVisualKeys(t *testing.T, patch dashboard.Patch, want []string) {
	t.Helper()
	got := make([]string, 0, len(patch.Visuals))
	for key := range patch.Visuals {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("visual keys = %#v, want %#v; status error = %q", got, want, patch.Status.Error)
	}
}

func hasDatumValue(rows []dashboard.Datum, key string, value string) bool {
	for _, row := range rows {
		if datumString(row, key) == value {
			return true
		}
	}
	return false
}

func hasHierarchyPathValue(rows []dashboard.Datum, value string) bool {
	for _, row := range rows {
		if strings.Contains(fmt.Sprint(row["path"]), value) {
			return true
		}
	}
	return false
}

func tableRowsHaveKey(rows []map[string]any, key string) bool {
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}

func tableRowsHaveValue(rows []map[string]any, key string) bool {
	for _, row := range rows {
		value, ok := row[key]
		if ok && value != nil && fmt.Sprint(value) != "" {
			return true
		}
	}
	return false
}

func tableColumnHasFormatting(columns []dashboard.TableColumn, key, kind string) bool {
	for _, column := range columns {
		if column.Key != key {
			continue
		}
		for _, rule := range column.Formatting {
			if rule.Kind == kind {
				return true
			}
		}
	}
	return false
}

func tableHasAnyFormatting(columns []dashboard.TableColumn, kind string) bool {
	for _, column := range columns {
		for _, rule := range column.Formatting {
			if rule.Kind == kind {
				return true
			}
		}
	}
	return false
}

func hasMetricDimension(dimensions []dashboard.MetricViewDimension, name, expr string) bool {
	for _, dimension := range dimensions {
		if dimension.Name == name && dimension.Expr == expr {
			return true
		}
	}
	return false
}

func hasMetricMeasure(measures []dashboard.MetricViewMeasure, name, expression string) bool {
	for _, measure := range measures {
		if measure.Name == name && measure.Expression == expression {
			return true
		}
	}
	return false
}

func hasModelNode(nodes []dashboard.ModelNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func hasModelEdge(edges []dashboard.ModelEdge, source, target string) bool {
	for _, edge := range edges {
		if edge.Source == source && edge.Target == target {
			return true
		}
	}
	return false
}

func datumString(row dashboard.Datum, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func datumInt(row dashboard.Datum, key string) int {
	value, ok := row[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		var out int
		_, _ = fmt.Sscan(fmt.Sprint(value), &out)
		return out
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
