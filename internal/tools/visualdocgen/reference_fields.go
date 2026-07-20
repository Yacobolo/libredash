package main

import (
	"fmt"

	"github.com/Yacobolo/leapview/internal/visualdocs"
)

var queryFieldReferences = map[string]visualdocs.FieldReference{
	"table": {
		Type:        "string",
		Description: "Selects the fact table when the semantic model cannot infer one from the referenced fields.",
	},
	"dimensions": {
		Type:        "field mapping",
		Description: "Groups query results and supplies category, hierarchy, matrix, graph, or geographic labels.",
	},
	"series": {
		Type:        "field reference",
		Description: "Splits one measure into named series for compatible chart shapes.",
	},
	"measures": {
		Type:        "measure mapping",
		Description: "Selects the governed semantic measures consumed by the visual shape.",
	},
	"time": {
		Type:        "time reference",
		Description: "Groups a time field at an explicit grain.",
	},
	"sort": {
		Type:        "sort list",
		Description: "Orders query results by a returned field or measure alias.",
	},
	"limit": {
		Type:          "integer",
		Default:       "no limit",
		AllowedValues: []string{"positive integer"},
		Description:   "Caps the number of rows returned to the renderer.",
	},
}

var optionFieldReferences = map[string]visualdocs.FieldReference{
	"area":           booleanOption("true", "Fills the radar polygon so its overall profile is easier to compare."),
	"bin_count":      field("integer", "20", []string{"5–60"}, "Controls the number of equal-width histogram bins."),
	"breadcrumb":     booleanOption("false", "Shows the treemap hierarchy breadcrumb."),
	"center_label":   field("string | boolean", "none", nil, "Adds a total or custom label to the center of a donut."),
	"curveness":      field("number", "renderer default", []string{"0–1"}, "Controls the curvature of graph or Sankey links."),
	"data_zoom":      booleanOption("false", "Adds inside and slider zoom controls to supported Cartesian charts."),
	"dual_axis":      booleanOption("false", "Places the second combo series on a separate value axis."),
	"focus":          field("string", "renderer default", []string{"adjacency", "descendant"}, "Selects which related graph or hierarchy elements receive emphasis."),
	"funnel_align":   field("string", "center", []string{"left", "center", "right"}, "Aligns funnel stages within the plotting area."),
	"initial_depth":  field("integer", "-1", []string{"-1 or greater"}, "Sets the deepest hierarchy level expanded initially; -1 expands all levels."),
	"label_position": field("string", "renderer default", []string{"top", "bottom", "left", "right", "inside", "outside"}, "Positions value labels relative to their marks."),
	"layout":         field("string", "force", []string{"force", "circular"}, "Selects the graph node layout algorithm."),
	"legend":         field("boolean | string", "false", []string{"true", "false", "top", "bottom", "left", "right"}, "Shows the legend and optionally selects its position."),
	"map":            field("string", "required", nil, "Names the registered geographic map used by the renderer."),
	"max":            field("number", "automatic", nil, "Sets the upper bound of a gauge scale."),
	"min":            field("number", "0", nil, "Sets the lower bound of a gauge scale."),
	"node_gap":       field("number", "8", []string{"0 or greater"}, "Sets the vertical gap between Sankey nodes."),
	"note":           field("string", "none", nil, "Adds supporting context below a KPI value."),
	"orient":         field("string", "renderer default", []string{"LR", "RL", "TB", "BT", "horizontal", "vertical"}, "Controls the direction of tree or Sankey layout."),
	"progress_width": field("number", "12", []string{"positive number"}, "Sets the width of the gauge progress arc."),
	"radius":         field("string | string tuple", "renderer default", nil, "Sets the outer radius or inner and outer radii of a pie or donut."),
	"roam":           booleanOption("renderer default", "Enables supported pan, zoom, or hierarchy navigation interactions."),
	"rose_type":      field("string", "none", []string{"radius", "area"}, "Scales pie sectors as a rose chart."),
	"series_types":   field("mapping", "automatic", []string{"line", "bar", "column"}, "Maps combo series names or measure aliases to renderer types."),
	"show_labels":    booleanOption("renderer default", "Shows value labels directly on chart marks."),
	"show_symbols":   booleanOption("true", "Shows point symbols on line and area series."),
	"smooth":         booleanOption("true", "Uses curved interpolation for line and area series."),
	"sort":           field("string", "descending", []string{"ascending", "descending", "none"}, "Controls the renderer-side ordering of funnel stages."),
	"stacked":        booleanOption("false", "Stacks compatible bar, column, line, or area series."),
	"step":           field("string | boolean", "false", []string{"start", "middle", "end", "true"}, "Draws line segments as discrete steps at the selected transition point."),
	"symbol_size":    field("number", "renderer default", []string{"positive number"}, "Sets point symbol size for line, area, and scatter series."),
	"thresholds":     field("threshold list", "none", nil, "Maps gauge thresholds to scale positions and colors."),
	"tone":           field("string", "neutral", []string{"neutral", "ink", "green", "amber", "coral"}, "Sets the semantic accent tone of a KPI card."),
}

func visualFieldReferences(queryFields, optionFields []string, chartType string) ([]visualdocs.FieldReference, error) {
	result := make([]visualdocs.FieldReference, 0, len(queryFields)+len(optionFields))
	for _, name := range queryFields {
		reference, ok := queryFieldReferences[name]
		if !ok {
			return nil, fmt.Errorf("query.%s has no documentation field metadata", name)
		}
		reference.Path = "query." + name
		result = append(result, reference)
	}
	for _, name := range optionFields {
		reference, ok := optionFieldReferences[name]
		if !ok {
			return nil, fmt.Errorf("options.%s has no documentation field metadata", name)
		}
		reference.Path = "options." + name
		reference.Default = visualOptionDefault(name, chartType, reference.Default)
		result = append(result, reference)
	}
	return result, nil
}

func visualOptionDefault(name, chartType, fallback string) string {
	switch name {
	case "curveness":
		if chartType == "graph" {
			return "0.18"
		}
		return "0.5"
	case "focus":
		if chartType == "tree" {
			return "descendant"
		}
		return "adjacency"
	case "orient":
		if chartType == "tree" {
			return "LR"
		}
		return "automatic"
	case "radius":
		if chartType == "donut" {
			return "48%, 72%"
		}
		return "0%, 72%"
	case "roam":
		switch chartType {
		case "graph", "map", "tree":
			return "true"
		default:
			return "false"
		}
	case "show_labels":
		switch chartType {
		case "pie", "donut", "funnel", "map":
			return "true"
		default:
			return "false"
		}
	case "smooth":
		if chartType == "line" || chartType == "area" {
			return "true"
		}
		return "false"
	case "symbol_size":
		if chartType == "scatter" {
			return "9"
		}
		return "7"
	default:
		return fallback
	}
}

func booleanOption(defaultValue, description string) visualdocs.FieldReference {
	return field("boolean", defaultValue, []string{"true", "false"}, description)
}

func field(valueType, defaultValue string, allowed []string, description string) visualdocs.FieldReference {
	return visualdocs.FieldReference{Type: valueType, Default: defaultValue, AllowedValues: allowed, Description: description}
}
