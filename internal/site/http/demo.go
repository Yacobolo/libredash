package http

import "github.com/Yacobolo/libredash/pkg/pagestream"

func chartShowcasePatch() pagestream.SignalPatch {
	return pagestream.SignalPatch{"charts": chartShowcase(), "tables": tableShowcase()}
}

func chartShowcase() []map[string]any {
	category := []map[string]any{{"label": "Jan", "value": 42}, {"label": "Feb", "value": 57}, {"label": "Mar", "value": 49}, {"label": "Apr", "value": 68}, {"label": "May", "value": 74}}
	partToWhole := []map[string]any{{"label": "Enterprise", "value": 48}, {"label": "Growth", "value": 31}, {"label": "Starter", "value": 21}}
	flow := []map[string]any{{"source": "Visit", "target": "Explore", "value": 120}, {"source": "Explore", "target": "Trial", "value": 76}, {"source": "Trial", "target": "Customer", "value": 42}, {"source": "Visit", "target": "Trial", "value": 18}}
	hierarchy := []map[string]any{{"path": "Revenue/Enterprise", "value": 48}, {"path": "Revenue/Growth", "value": 31}, {"path": "Revenue/Starter", "value": 21}}
	return []map[string]any{
		showcaseChart("line", "Line", "category_value", "line", category, nil),
		showcaseChart("area", "Area", "category_value", "area", category, nil),
		showcaseChart("bar", "Bar", "category_value", "bar", category, nil),
		showcaseChart("column", "Column", "category_value", "column", category, nil),
		showcaseChart("pie", "Pie", "category_value", "pie", partToWhole, map[string]any{"show_labels": true}),
		showcaseChart("donut", "Donut", "category_value", "donut", partToWhole, map[string]any{"center_label": "Segments"}),
		showcaseChart("scatter", "Scatter", "category_value", "scatter", category, nil),
		showcaseChart("funnel", "Funnel", "category_value", "funnel", []map[string]any{{"label": "Visit", "value": 120}, {"label": "Explore", "value": 94}, {"label": "Trial", "value": 61}, {"label": "Customer", "value": 42}}, nil),
		showcaseChart("treemap", "Treemap", "category_value", "treemap", partToWhole, nil),
		showcaseChart("gauge", "Gauge", "single_value", "gauge", []map[string]any{{"label": "Target", "value": 72}}, map[string]any{"max": 100}),
		showcaseChart("heatmap", "Heatmap", "matrix", "heatmap", []map[string]any{{"row": "North", "column": "Jan", "value": 24}, {"row": "North", "column": "Feb", "value": 52}, {"row": "South", "column": "Jan", "value": 66}, {"row": "South", "column": "Feb", "value": 38}}, nil),
		showcaseChart("sankey", "Sankey", "graph", "sankey", flow, nil),
		showcaseChart("graph", "Graph", "graph", "graph", flow, map[string]any{"roam": false}),
		showcaseChart("map", "Map", "geo", "map", []map[string]any{{"name": "SP", "value": 88}, {"name": "RJ", "value": 64}, {"name": "MG", "value": 52}, {"name": "BA", "value": 39}}, map[string]any{"map": "brazil_states", "roam": false}),
		showcaseChart("candlestick", "Candlestick", "ohlc", "candlestick", []map[string]any{{"label": "Mon", "open": 42, "close": 48, "low": 39, "high": 51}, {"label": "Tue", "open": 48, "close": 45, "low": 43, "high": 50}, {"label": "Wed", "open": 45, "close": 54, "low": 44, "high": 56}, {"label": "Thu", "open": 54, "close": 58, "low": 51, "high": 61}}, nil),
		showcaseChart("boxplot", "Boxplot", "distribution", "boxplot", []map[string]any{{"label": "North", "min": 12, "q1": 22, "median": 32, "q3": 48, "max": 60}, {"label": "South", "min": 18, "q1": 28, "median": 42, "q3": 54, "max": 72}}, nil),
		showcaseChart("combo", "Combo", "category_multi_measure", "combo", []map[string]any{{"label": "Jan", "series": "Revenue", "value": 42}, {"label": "Jan", "series": "Margin", "value": 28}, {"label": "Feb", "series": "Revenue", "value": 57}, {"label": "Feb", "series": "Margin", "value": 35}, {"label": "Mar", "series": "Revenue", "value": 49}, {"label": "Mar", "series": "Margin", "value": 31}}, map[string]any{"series_types": map[string]any{"Revenue": "bar", "Margin": "line"}}),
		showcaseChart("waterfall", "Waterfall", "category_delta", "waterfall", []map[string]any{{"label": "Start", "start": 0, "value": 120}, {"label": "Expansion", "start": 120, "value": 32}, {"label": "Churn", "start": 128, "value": -24}, {"label": "End", "start": 0, "value": 128}}, nil),
		showcaseChart("histogram", "Histogram", "binned_measure", "histogram", []map[string]any{{"label": "0–10", "value": 12}, {"label": "10–20", "value": 28}, {"label": "20–30", "value": 46}, {"label": "30–40", "value": 31}, {"label": "40–50", "value": 16}}, nil),
		showcaseChart("radar", "Radar", "category_value", "radar", []map[string]any{{"label": "Reach", "value": 78}, {"label": "Speed", "value": 64}, {"label": "Quality", "value": 82}, {"label": "Cost", "value": 53}, {"label": "Trust", "value": 71}}, nil),
		showcaseChart("tree", "Tree", "hierarchy", "tree", hierarchy, map[string]any{"roam": false}),
		showcaseChart("sunburst", "Sunburst", "hierarchy", "sunburst", hierarchy, nil),
		showcaseChart("kpi", "KPI", "single_value", "kpi", []map[string]any{{"label": "Active workspaces", "value": 128}}, map[string]any{"tone": "green", "note": "This month"}),
	}
}

func showcaseChart(id, title, shape, chartType string, data []map[string]any, options map[string]any) map[string]any {
	renderer := "echarts"
	kind := "chart"
	if chartType == "kpi" {
		renderer = "html"
		kind = "kpi"
	}
	return map[string]any{
		"version":  3,
		"id":       id,
		"kind":     kind,
		"shape":    shape,
		"renderer": renderer,
		"type":     chartType,
		"title":    title,
		"data":     data,
		"options":  options,
	}
}

func tableShowcase() []map[string]any {
	orders := []map[string]any{
		{"order_id": "LD-10482", "purchase_date": "2024-06-14", "status": "delivered", "state": "SP", "category": "Electronics", "revenue": 489.30, "review_score": 4.8, "delivery_days": 3},
		{"order_id": "LD-10481", "purchase_date": "2024-06-13", "status": "shipped", "state": "RJ", "category": "Home & living", "revenue": 284.00, "review_score": 4.2, "delivery_days": 6},
		{"order_id": "LD-10480", "purchase_date": "2024-06-12", "status": "processing", "state": "MG", "category": "Beauty", "revenue": 118.50, "review_score": 3.6, "delivery_days": 11},
		{"order_id": "LD-10479", "purchase_date": "2024-06-11", "status": "delivered", "state": "BA", "category": "Sports", "revenue": 356.75, "review_score": 4.9, "delivery_days": 4},
		{"order_id": "LD-10478", "purchase_date": "2024-06-10", "status": "unavailable", "state": "PR", "category": "Books", "revenue": 72.20, "review_score": 2.7, "delivery_days": 22},
		{"order_id": "LD-10477", "purchase_date": "2024-06-09", "status": "canceled", "state": "RS", "category": "Fashion", "revenue": 154.90, "review_score": 2.1, "delivery_days": 29},
	}
	return []map[string]any{
		showcaseTable("orders-table", "Orders", "data_table", tableStyle("comfortable", true, "rows"), "purchase_date", "desc", orderColumns(), orders),
		showcaseTable("orders-compact", "Orders compact", "data_table", tableStyle("compact", false, "rows"), "purchase_date", "desc", []map[string]any{
			tableColumn("order_id", "Order", "", 200, "text", nil), tableColumn("status", "Status", "", 116, "text", nil), tableColumn("state", "State", "", 70, "text", nil), tableColumn("revenue", "Revenue", "right", 116, "currency", nil), tableColumn("review_score", "Review", "right", 92, "decimal", nil),
		}, orders),
		showcaseTable("orders-spacious", "Orders spacious", "data_table", tableStyle("spacious", true, "rows"), "purchase_date", "desc", []map[string]any{
			tableColumn("order_id", "Order", "", 250, "text", nil), tableColumn("purchase_date", "Purchased", "", 136, "text", nil), tableColumn("category", "Category", "", 230, "text", nil), tableColumn("revenue", "Revenue", "right", 140, "currency", nil), tableColumn("delivery_days", "Delivery", "right", 116, "days", nil),
		}, orders),
		showcaseTable("orders-full-grid", "Orders full grid", "data_table", tableStyle("comfortable", true, "full"), "revenue", "desc", []map[string]any{
			tableColumn("order_id", "Order", "", 220, "text", nil), tableColumn("status", "Status", "", 122, "text", nil), tableColumn("state", "State", "", 76, "text", nil), tableColumn("category", "Category", "", 190, "text", nil), tableColumn("revenue", "Revenue", "right", 128, "currency", nil), tableColumn("delivery_days", "Delivery", "right", 110, "days", nil),
		}, orders),
		showcaseTable("orders-conditional", "Orders conditional formatting", "data_table", tableStyle("comfortable", true, "full"), "revenue", "desc", []map[string]any{
			tableColumn("order_id", "Order", "", 212, "text", nil),
			tableColumn("status", "Status", "", 132, "text", []map[string]any{{"kind": "badge", "values": map[string]string{"delivered": "success", "shipped": "accent", "processing": "accent", "unavailable": "warning", "canceled": "danger"}}}),
			tableColumn("revenue", "Revenue", "right", 136, "currency", []map[string]any{{"kind": "data_bar", "min": 0, "max": 500, "color": "accent"}}),
			tableColumn("review_score", "Review", "right", 102, "decimal", []map[string]any{{"kind": "text_color", "min": 4, "color": "success"}, {"kind": "text_color", "min": 3, "max": 3.99, "color": "warning"}, {"kind": "text_color", "max": 2.99, "color": "danger"}}),
			tableColumn("delivery_days", "Delivery", "right", 112, "days", []map[string]any{{"kind": "background_scale", "min": 0, "max": 30, "highColor": "danger"}}),
		}, orders),
		showcaseTable("state-status-matrix", "State status matrix", "matrix_table", tableStyle("comfortable", true, "rows"), "state", "asc", []map[string]any{
			tableHeader("state", "State", "row_header", "", "", "", 76, "text", nil),
			tableHeader("pivot_delivered__order_count", "Orders", "measure", "Delivered", "order_count", "Delivered", 104, "integer", nil), tableHeader("pivot_delivered__revenue", "Revenue", "measure", "Delivered", "revenue", "Delivered", 124, "currency", nil),
			tableHeader("pivot_shipped__order_count", "Orders", "measure", "Shipped", "order_count", "Shipped", 104, "integer", nil), tableHeader("pivot_shipped__revenue", "Revenue", "measure", "Shipped", "revenue", "Shipped", 124, "currency", nil),
		}, []map[string]any{{"state": "SP", "pivot_delivered__order_count": 182, "pivot_delivered__revenue": 24800, "pivot_shipped__order_count": 46, "pivot_shipped__revenue": 6180}, {"state": "RJ", "pivot_delivered__order_count": 126, "pivot_delivered__revenue": 16300, "pivot_shipped__order_count": 38, "pivot_shipped__revenue": 4920}, {"state": "MG", "pivot_delivered__order_count": 94, "pivot_delivered__revenue": 11950, "pivot_shipped__order_count": 31, "pivot_shipped__revenue": 3710}}),
		showcaseTable("category-status-pivot", "Category status pivot", "pivot_table", tableStyle("comfortable", true, "rows"), "category", "asc", []map[string]any{
			tableHeader("category", "Category", "row_header", "", "", "", 170, "text", nil), tableHeader("pivot_delivered", "Delivered", "measure", "Orders", "order_count", "Delivered", 110, "integer", nil), tableHeader("pivot_shipped", "Shipped", "measure", "Orders", "order_count", "Shipped", 104, "integer", nil), tableHeader("pivot_canceled", "Canceled", "measure", "Orders", "order_count", "Canceled", 108, "integer", nil),
		}, []map[string]any{{"category": "Electronics", "pivot_delivered": 138, "pivot_shipped": 24, "pivot_canceled": 7}, {"category": "Home & living", "pivot_delivered": 112, "pivot_shipped": 31, "pivot_canceled": 4}, {"category": "Beauty", "pivot_delivered": 86, "pivot_shipped": 19, "pivot_canceled": 9}, {"category": "Sports", "pivot_delivered": 74, "pivot_shipped": 16, "pivot_canceled": 3}}),
		showcaseTable("state-status-matrix-formatted", "State/category matrix formatted", "matrix_table", tableStyle("comfortable", true, "full"), "state", "asc", []map[string]any{
			tableHeader("state", "State", "row_header", "", "", "", 76, "text", nil), tableHeader("category", "Category", "row_header", "", "", "", 190, "text", nil), tableHeader("order_count", "Orders", "measure", "Measures", "order_count", "", 120, "integer", []map[string]any{{"kind": "data_bar", "min": 0, "max": 250, "color": "accent"}}), tableHeader("revenue", "Revenue", "measure", "Measures", "revenue", "", 132, "currency", []map[string]any{{"kind": "data_bar", "min": 0, "max": 20000, "color": "success"}}),
		}, []map[string]any{{"state": "SP", "category": "Electronics", "order_count": 214, "revenue": 28640}, {"state": "SP", "category": "Home & living", "order_count": 168, "revenue": 19410}, {"state": "RJ", "category": "Beauty", "order_count": 116, "revenue": 12780}, {"state": "MG", "category": "Sports", "order_count": 89, "revenue": 10340}}),
		showcaseTable("category-status-pivot-heat", "Category status pivot heat", "pivot_table", tableStyle("compact", true, "full"), "category", "asc", []map[string]any{
			tableHeader("category", "Category", "row_header", "", "", "", 170, "text", nil), tableHeader("pivot_delivered", "Delivered", "measure", "Orders", "order_count", "Delivered", 110, "integer", []map[string]any{{"kind": "background_scale", "min": 0, "max": 200, "highColor": "accent"}}), tableHeader("pivot_shipped", "Shipped", "measure", "Orders", "order_count", "Shipped", 104, "integer", []map[string]any{{"kind": "background_scale", "min": 0, "max": 200, "highColor": "accent"}}), tableHeader("pivot_canceled", "Canceled", "measure", "Orders", "order_count", "Canceled", 108, "integer", []map[string]any{{"kind": "background_scale", "min": 0, "max": 200, "highColor": "accent"}}),
		}, []map[string]any{{"category": "Electronics", "pivot_delivered": 138, "pivot_shipped": 24, "pivot_canceled": 7}, {"category": "Home & living", "pivot_delivered": 112, "pivot_shipped": 31, "pivot_canceled": 4}, {"category": "Beauty", "pivot_delivered": 86, "pivot_shipped": 19, "pivot_canceled": 9}, {"category": "Sports", "pivot_delivered": 74, "pivot_shipped": 16, "pivot_canceled": 3}}),
	}
}

func orderColumns() []map[string]any {
	return []map[string]any{
		tableColumn("order_id", "Order", "", 240, "text", nil), tableColumn("purchase_date", "Purchased", "", 126, "text", nil), tableColumn("status", "Status", "", 128, "text", nil), tableColumn("state", "State", "", 78, "text", nil), tableColumn("category", "Category", "", 210, "text", nil), tableColumn("revenue", "Revenue", "right", 130, "currency", nil), tableColumn("review_score", "Review", "right", 104, "decimal", nil), tableColumn("delivery_days", "Delivery", "right", 108, "days", nil),
	}
}

func tableStyle(density string, zebra bool, grid string) map[string]any {
	return map[string]any{"density": density, "zebra": zebra, "grid": grid}
}

func tableColumn(key, label, align string, width int, format string, formatting []map[string]any) map[string]any {
	return tableHeader(key, label, "", "", "", "", width, format, formatting, map[string]any{"align": align})
}

func tableHeader(key, label, role, group, measure, columnValue string, width int, format string, formatting []map[string]any, extra ...map[string]any) map[string]any {
	column := map[string]any{"key": key, "label": label, "role": role, "group": group, "measure": measure, "columnValue": columnValue, "width": width, "format": format}
	if len(formatting) > 0 {
		column["formatting"] = formatting
	}
	for _, values := range extra {
		for name, value := range values {
			if value != "" {
				column[name] = value
			}
		}
	}
	return column
}

func showcaseTable(id, title, kind string, style map[string]any, sortKey, direction string, columns, rows []map[string]any) map[string]any {
	chunkSize := 50
	return map[string]any{
		"version": 2, "id": id, "kind": kind, "title": title, "style": style, "interaction": map[string]any{}, "selection": []map[string]any{}, "columns": columns,
		"totalRows": len(rows), "cardinality": map[string]any{"kind": "exact", "value": len(rows)}, "availableRows": len(rows), "isCapped": false, "rowCap": 10000, "chunkSize": chunkSize, "rowHeight": 34, "resetVersion": 0,
		"sort":         map[string]any{"key": sortKey, "direction": direction},
		"blocks":       map[string]any{"a": map[string]any{"start": 0, "requestSeq": 0, "resetVersion": 0, "sort": map[string]any{"key": sortKey, "direction": direction}, "rows": rows}},
		"loadingBlock": "", "error": "",
	}
}
