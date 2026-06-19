package data

import (
	"strconv"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	"github.com/Yacobolo/libredash/internal/semantic"
)

func modelGraph(model *semantic.Model, metricViews map[string]*semantic.MetricView) dashboard.ModelGraph {
	graph := dashboard.ModelGraph{
		Name:  model.Name,
		Title: model.Title,
		Stats: dashboard.ModelStats{
			Sources:       len(model.Sources),
			ModelTables:   len(model.Tables),
			Metrics:       measureCount(model.Name, metricViews),
			Visuals:       0,
			ReportTables:  0,
			Relationships: len(model.Relationships),
		},
	}

	for _, name := range sortedKeys(model.Sources) {
		source := model.Sources[name]
		sourceKind := source.Kind()
		meta := []dashboard.ModelMeta{
			{Label: "Kind", Value: sourceKind},
			{Label: "Schema", Value: "raw"},
		}
		if source.Format != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Format", Value: source.Format})
		}
		if source.Path != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Path", Value: source.Path})
		}
		if source.Object != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Object", Value: source.Object})
		}
		if source.Connection != "" {
			meta = append(meta, dashboard.ModelMeta{Label: "Connection", Value: source.Connection})
			if connection, ok := model.Connections[source.Connection]; ok {
				meta = append(meta, dashboard.ModelMeta{Label: "Connection Kind", Value: connection.Kind})
			}
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:     nodeID("source", name),
			Label:  name,
			Kind:   "source",
			Schema: "raw",
			Fields: []dashboard.ModelField{{Name: source.Description(), Role: source.Role()}},
			Meta:   meta,
		})
	}

	for _, name := range sortedKeys(model.Tables) {
		table := model.Tables[name]
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:          nodeID("model_table", name),
			Label:       name,
			Kind:        "model_table",
			Schema:      "model",
			Description: table.Description,
			Fields:      modelTableFields(table),
			Meta: []dashboard.ModelMeta{
				{Label: "Mode", Value: "DuckDB import"},
				{Label: "Kind", Value: table.Kind},
				{Label: "Grain", Value: table.Grain},
				{Label: "Schema", Value: "model"},
			},
		})
		if table.Source != "" {
			graph.Edges = append(graph.Edges, dashboard.ModelEdge{
				ID:     "source_" + table.Source + "_to_model_table_" + name,
				Source: nodeID("source", table.Source),
				Target: nodeID("model_table", name),
				Label:  "backs",
				Kind:   "materialization",
			})
		}
	}

	for _, relationship := range model.Relationships {
		fromTable, fromField := modelEndpoint(relationship.From)
		toTable, toField := modelEndpoint(relationship.To)
		graph.Edges = append(graph.Edges, dashboard.ModelEdge{
			ID:          relationship.ID,
			Source:      nodeID("model_table", fromTable),
			Target:      nodeID("model_table", toTable),
			Label:       fromField + " -> " + toField,
			Kind:        "relationship",
			SourceField: fromField,
			TargetField: toField,
			Cardinality: relationship.Cardinality,
		})
	}

	for _, name := range sortedKeys(metricViews) {
		view := metricViews[name]
		if view.SemanticModel != model.Name {
			continue
		}
		fields := make([]dashboard.ModelField, 0, len(view.Dimensions)+len(view.Measures))
		for _, dimension := range sortedKeys(view.Dimensions) {
			fields = append(fields, dashboard.ModelField{Name: dimension, Role: "dimension"})
		}
		for _, measure := range sortedKeys(view.Measures) {
			fields = append(fields, dashboard.ModelField{Name: measure, Role: "measure"})
		}
		graph.Nodes = append(graph.Nodes, dashboard.ModelNode{
			ID:          nodeID("metric_view", name),
			Label:       view.Title,
			Kind:        "metric_view",
			Schema:      "metrics",
			Description: view.Description,
			Fields:      fields,
			Meta: []dashboard.ModelMeta{
				{Label: "Base table", Value: view.BaseTable},
				{Label: "Time", Value: view.Time.DefaultField},
				{Label: "Dimensions", Value: strconv.Itoa(len(view.Dimensions))},
				{Label: "Measures", Value: strconv.Itoa(len(view.Measures))},
			},
		})
		graph.Edges = append(graph.Edges, dashboard.ModelEdge{
			ID:     "metric_view_" + name + "_from_" + view.BaseTable,
			Source: nodeID("model_table", view.BaseTable),
			Target: nodeID("metric_view", name),
			Label:  "metric view",
			Kind:   "metrics",
		})
	}

	return graph
}

func nodeID(kind, name string) string {
	return kind + ":" + name
}

func modelEndpoint(path string) (string, string) {
	parts := strings.Split(path, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	if len(parts) < 2 {
		return path, ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

func modelTableFields(table semantic.ModelTable) []dashboard.ModelField {
	fields := []dashboard.ModelField{}
	for _, name := range sortedKeys(table.Dimensions) {
		role := "dimension"
		if name == table.PrimaryKey {
			role = "key"
		}
		fields = append(fields, dashboard.ModelField{Name: name, Role: role})
	}
	for _, name := range sortedKeys(table.Measures) {
		fields = append(fields, dashboard.ModelField{Name: name, Role: "measure"})
	}
	return fields
}

func measureCount(modelID string, metricViews map[string]*semantic.MetricView) int {
	count := 0
	for _, view := range metricViews {
		if view.SemanticModel == modelID {
			count += len(view.Measures)
		}
	}
	return count
}
