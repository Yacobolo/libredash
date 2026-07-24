package report

import (
	"fmt"
	"strings"
)

func (d *Dashboard) ValidateContract() error {
	return d.validateContract()
}

func (d *Dashboard) validateContract() error {
	if d.ID == "" || d.Title == "" {
		return fmt.Errorf("dashboard requires id and title")
	}
	if d.SemanticModel == "" {
		return fmt.Errorf("dashboard %q requires semantic_model", d.ID)
	}
	if len(d.Visuals) == 0 {
		return fmt.Errorf("dashboard %q requires visuals", d.ID)
	}
	if len(d.Pages) == 0 {
		return fmt.Errorf("dashboard %q requires pages", d.ID)
	}
	if err := d.validateFilterArchitectureContract(); err != nil {
		return err
	}
	for name, authored := range d.Visuals {
		if (authored.Chart == nil) == (authored.Tabular == nil) {
			return fmt.Errorf("visual %q must contain exactly one authoring variant", name)
		}
		if authored.Chart != nil {
			if err := d.validateChartContract(name, *authored.Chart); err != nil {
				return err
			}
			continue
		}
		if err := d.validateTabularContract(name, authored.Type, *authored.Tabular); err != nil {
			return err
		}
	}
	return d.validatePages()
}

func (d *Dashboard) validateChartContract(name string, visual Visual) error {
	kind := visual.KindOrDefault()
	if kind != "kpi" && visual.Title == "" {
		return fmt.Errorf("visual %q requires title", name)
	}
	if kind != "kpi" && visual.Type == "" {
		return fmt.Errorf("visual %q requires type", name)
	}
	if !supportsVisualKind(kind) {
		return fmt.Errorf("visual %q has unsupported kind %q", name, kind)
	}
	shape := visual.ResultShape()
	renderer := visual.ownedRenderer()
	if !supportsVisualShape(shape) {
		return fmt.Errorf("visual %q has unsupported shape %q", name, shape)
	}
	if !rendererSupportsType(renderer, visual.Type) {
		return fmt.Errorf("visual %q has unsupported type %q", name, visual.Type)
	}
	if !rendererSupportsShapeType(renderer, shape, visual.Type) {
		return fmt.Errorf("visual %q type %q does not support data shape %q", name, visual.Type, shape)
	}
	if err := validateVisualQueryShape(name, visual); err != nil {
		return err
	}
	if err := validateVisualPresentation(name, visual); err != nil {
		return err
	}
	if !visual.Query.Series.IsZero() {
		if !supportsSeries(shape) {
			return fmt.Errorf("visual %q shape %q does not support series", name, shape)
		}
		if !rendererTypeSupportsSeries(renderer, visual.Type) {
			return fmt.Errorf("visual %q type %q does not support series", name, visual.Type)
		}
	}
	if shape == "geo" {
		if err := validateGeographicVisual(name, visual); err != nil {
			return err
		}
	}
	if visual.Type == "custom" && (visual.Custom.Engine != "vega_lite" || len(visual.Custom.Program) == 0) {
		return fmt.Errorf("visual %q custom visualization requires a non-empty vega_lite program", name)
	}
	for _, sort := range visual.Query.Sort {
		if sort.Field == "" && sort.Expr == "" {
			return fmt.Errorf("visual %q has sort missing field or expr", name)
		}
	}
	if !visual.Interaction.RowSelection.IsZero() {
		return fmt.Errorf("visual %q does not support row_selection", name)
	}
	if !visual.Interaction.PointSelection.IsZero() {
		if kind == "kpi" {
			return fmt.Errorf("visual %q kind kpi does not support point_selection", name)
		}
		if err := d.validateSelectionInteraction("visual", name, "point_selection", visual.Interaction.PointSelection); err != nil {
			return err
		}
	}
	if !visual.Interaction.SpatialSelection.IsZero() {
		if visual.Type != "map" {
			return fmt.Errorf("visual %q type %q does not support spatial_selection", name, visual.Type)
		}
		if err := validateSpatialSelectionInteraction(name, visual); err != nil {
			return err
		}
		for _, target := range visual.Interaction.SpatialSelection.Targets {
			if err := d.validateInteractionTarget("visual", name, "spatial_selection", target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Dashboard) validateTabularContract(name, visualType string, table TableVisual) error {
	if table.Title == "" {
		return fmt.Errorf("table %q requires title", name)
	}
	if err := validateTableStyle(name, table.Style); err != nil {
		return err
	}
	switch table.CardinalityOrDefault() {
	case TableCardinalityBounded, TableCardinalityExact:
	default:
		return fmt.Errorf("table %q has unsupported cardinality %q", name, table.Cardinality)
	}
	for _, column := range table.Columns {
		if err := validateTableColumn(name, column); err != nil {
			return err
		}
	}
	for measure, rules := range table.MeasureFormatting {
		for _, rule := range rules {
			if err := validateTableFormattingRule(name, measure, rule); err != nil {
				return err
			}
		}
	}
	switch visualType {
	case "table":
		if table.Query.Table == "" {
			return fmt.Errorf("table %q type table requires query.table", name)
		}
		if len(table.Query.Fields) == 0 && len(table.Query.Columns) == 0 {
			return fmt.Errorf("table %q type table requires query.fields or query.columns", name)
		}
	case "matrix":
		if !table.Interaction.RowSelection.IsZero() {
			return fmt.Errorf("table %q type matrix does not support row_selection", name)
		}
		if len(table.Query.Rows) == 0 || len(table.Query.Measures) == 0 {
			return fmt.Errorf("table %q type matrix requires query.rows and query.measures", name)
		}
		if len(table.Query.Columns) > 1 {
			return fmt.Errorf("table %q type matrix supports at most one column dimension", name)
		}
	case "pivot":
		if !table.Interaction.RowSelection.IsZero() {
			return fmt.Errorf("table %q type pivot does not support row_selection", name)
		}
		if len(table.Query.Rows) == 0 || len(table.Query.Columns) != 1 || len(table.Query.Measures) != 1 {
			return fmt.Errorf("table %q type pivot requires query.rows, one query column dimension, and one query measure", name)
		}
	default:
		return fmt.Errorf("visual %q has unsupported tabular type %q", name, visualType)
	}
	if !table.Interaction.PointSelection.IsZero() {
		return fmt.Errorf("table %q does not support point_selection", name)
	}
	if !table.Interaction.SpatialSelection.IsZero() {
		return fmt.Errorf("table %q does not support spatial_selection", name)
	}
	if !table.Interaction.RowSelection.IsZero() {
		if err := d.validateSelectionInteraction("visual", name, "row_selection", table.Interaction.RowSelection); err != nil {
			return err
		}
	}
	return nil
}

func validateGeographicVisual(name string, visual Visual) error {
	if len(visual.Geo.Layers) == 0 {
		return fmt.Errorf("visual %q geographic visualization requires geo.layers", name)
	}
	aliases := map[string]struct{}{}
	for _, field := range visual.Query.Dimensions {
		aliases[defaultString(field.Alias, fieldRefAlias(field.Field))] = struct{}{}
	}
	if visual.Query.Time.Field != "" {
		aliases[defaultString(visual.Query.Time.Alias, fieldRefAlias(visual.Query.Time.Field))] = struct{}{}
	}
	for _, field := range visual.Query.Measures {
		aliases[defaultString(field.Alias, fieldRefAlias(field.Field))] = struct{}{}
	}
	requireAlias := func(layerID, property, alias string) error {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("visual %q geographic layer %q requires %s", name, layerID, property)
		}
		if _, ok := aliases[alias]; !ok {
			return fmt.Errorf("visual %q geographic layer %q %s references unknown query alias %q", name, layerID, property, alias)
		}
		return nil
	}
	optionalAlias := func(layerID, property, alias string) error {
		if strings.TrimSpace(alias) == "" {
			return nil
		}
		return requireAlias(layerID, property, alias)
	}
	if !oneOf(visual.Geo.Theme, "", "auto", "light", "dark") {
		return fmt.Errorf("visual %q has unsupported geo.theme %q", name, visual.Geo.Theme)
	}
	if !oneOf(visual.Geo.LabelDensity, "", "hidden", "normal", "dense") {
		return fmt.Errorf("visual %q has unsupported geo.label_density %q", name, visual.Geo.LabelDensity)
	}
	if !oneOf(visual.Geo.Camera.Mode, "", "fit_data", "fixed", "preserve") {
		return fmt.Errorf("visual %q has unsupported geo.camera.mode %q", name, visual.Geo.Camera.Mode)
	}
	if len(visual.Geo.Camera.Center) != 0 && len(visual.Geo.Camera.Center) != 2 {
		return fmt.Errorf("visual %q geo.camera.center requires longitude and latitude", name)
	}
	if len(visual.Geo.Camera.Center) == 2 && (visual.Geo.Camera.Center[0] < -180 || visual.Geo.Camera.Center[0] > 180 || visual.Geo.Camera.Center[1] < -90 || visual.Geo.Camera.Center[1] > 90) {
		return fmt.Errorf("visual %q geo.camera.center is outside geographic bounds", name)
	}
	if visual.Geo.Camera.MinimumZoom < 0 || visual.Geo.Camera.MaximumZoom < 0 || (visual.Geo.Camera.MaximumZoom > 0 && visual.Geo.Camera.MinimumZoom > visual.Geo.Camera.MaximumZoom) {
		return fmt.Errorf("visual %q has invalid geo.camera zoom range", name)
	}
	seen := map[string]struct{}{}
	for _, layer := range visual.Geo.Layers {
		if strings.TrimSpace(layer.ID) == "" {
			return fmt.Errorf("visual %q geographic layer requires id", name)
		}
		if _, exists := seen[layer.ID]; exists {
			return fmt.Errorf("visual %q has duplicate geographic layer %q", name, layer.ID)
		}
		seen[layer.ID] = struct{}{}
		if layer.Value != "" {
			if err := requireAlias(layer.ID, "value", layer.Value); err != nil {
				return err
			}
		}
		for property, alias := range map[string]string{"category": layer.Category, "label": layer.Label, "path": layer.Path, "order": layer.Order} {
			if err := optionalAlias(layer.ID, property, alias); err != nil {
				return err
			}
		}
		for _, alias := range layer.Tooltip {
			if err := requireAlias(layer.ID, "tooltip", alias); err != nil {
				return err
			}
		}
		if !oneOf(layer.Position, "", "below_labels", "above_labels") {
			return fmt.Errorf("visual %q geographic layer %q has unsupported position %q", name, layer.ID, layer.Position)
		}
		if layer.Visibility.MinimumZoom < 0 || layer.Visibility.MaximumZoom < 0 || (layer.Visibility.MaximumZoom > 0 && layer.Visibility.MinimumZoom > layer.Visibility.MaximumZoom) {
			return fmt.Errorf("visual %q geographic layer %q has invalid visibility zoom range", name, layer.ID)
		}
		if layer.Size.MinimumRadius < 0 || layer.Size.MaximumRadius < 0 || (layer.Size.MaximumRadius > 0 && layer.Size.MinimumRadius > layer.Size.MaximumRadius) {
			return fmt.Errorf("visual %q geographic layer %q size minimum_radius must not exceed maximum_radius", name, layer.ID)
		}
		if layer.Size.DomainMinimum != nil && layer.Size.DomainMaximum != nil && *layer.Size.DomainMinimum >= *layer.Size.DomainMaximum {
			return fmt.Errorf("visual %q geographic layer %q has invalid size domain", name, layer.ID)
		}
		if !oneOf(layer.Color.Kind, "", "sequential", "diverging", "categorical") {
			return fmt.Errorf("visual %q geographic layer %q has unsupported color kind %q", name, layer.ID, layer.Color.Kind)
		}
		if layer.Color.DomainMinimum != nil && layer.Color.DomainMaximum != nil && *layer.Color.DomainMinimum >= *layer.Color.DomainMaximum {
			return fmt.Errorf("visual %q geographic layer %q has invalid color domain", name, layer.ID)
		}
		if layer.Opacity < 0 || layer.Opacity > 1 || layer.Stroke.Opacity < 0 || layer.Stroke.Opacity > 1 {
			return fmt.Errorf("visual %q geographic layer %q opacity must be between zero and one", name, layer.ID)
		}
		if layer.Cluster.Enabled && layer.Kind != "point" && oneOf(layer.Kind, "choropleth", "heat", "density", "reference", "path") {
			return fmt.Errorf("visual %q geographic layer %q clustering is only supported for point layers", name, layer.ID)
		}
		switch layer.Kind {
		case "choropleth":
			if strings.TrimSpace(layer.GeometryAsset) == "" {
				return fmt.Errorf("visual %q choropleth layer %q requires geometry_asset", name, layer.ID)
			}
			if err := requireAlias(layer.ID, "join", layer.Join); err != nil {
				return err
			}
			if layer.Latitude != "" || layer.Longitude != "" {
				return fmt.Errorf("visual %q choropleth layer %q does not accept latitude or longitude", name, layer.ID)
			}
		case "point", "heat", "density", "path":
			if layer.GeometryAsset != "" || layer.Join != "" {
				return fmt.Errorf("visual %q geographic layer %q kind %q does not accept geometry_asset or join", name, layer.ID, layer.Kind)
			}
			if err := requireAlias(layer.ID, "latitude", layer.Latitude); err != nil {
				return err
			}
			if err := requireAlias(layer.ID, "longitude", layer.Longitude); err != nil {
				return err
			}
			if layer.Kind == "path" {
				if err := requireAlias(layer.ID, "path", layer.Path); err != nil {
					return err
				}
				if err := requireAlias(layer.ID, "order", layer.Order); err != nil {
					return err
				}
			}
		case "reference":
			if strings.TrimSpace(layer.GeometryAsset) == "" {
				return fmt.Errorf("visual %q reference layer %q requires geometry_asset", name, layer.ID)
			}
			if layer.Join != "" || layer.Latitude != "" || layer.Longitude != "" || layer.Value != "" || layer.Category != "" {
				return fmt.Errorf("visual %q reference layer %q does not accept query field bindings", name, layer.ID)
			}
		default:
			return fmt.Errorf("visual %q geographic layer %q has unsupported kind %q", name, layer.ID, layer.Kind)
		}
	}
	return nil
}

func validateVisualPresentation(name string, visual Visual) error {
	presentation := visual.Presentation
	if !oneOf(presentation.Legend, "", "hidden", "top", "right", "bottom", "left") {
		return fmt.Errorf("visual %q has unsupported presentation.legend %q", name, presentation.Legend)
	}
	if !oneOf(presentation.Orientation, "", "horizontal", "vertical") {
		return fmt.Errorf("visual %q has unsupported presentation.orientation %q", name, presentation.Orientation)
	}
	if !oneOf(presentation.LabelPosition, "", "automatic", "inside", "outside", "top") {
		return fmt.Errorf("visual %q has unsupported presentation.label_position %q", name, presentation.LabelPosition)
	}
	if !oneOf(presentation.Tone, "", "neutral", "ink", "success", "warning", "danger") {
		return fmt.Errorf("visual %q has unsupported presentation.tone %q", name, presentation.Tone)
	}
	if presentation.HistogramBins > 0 && visual.Type != "histogram" {
		return fmt.Errorf("visual %q presentation.histogram_bins is only valid for histogram", name)
	}
	if len(presentation.SeriesTypes) > 0 && visual.Type != "combo" {
		return fmt.Errorf("visual %q presentation.series_types is only valid for combo", name)
	}
	if presentation.DualAxis && visual.Type != "combo" {
		return fmt.Errorf("visual %q presentation.dual_axis is only valid for combo", name)
	}
	if presentation.Basemap != "" && visual.Type != "map" {
		return fmt.Errorf("visual %q presentation.basemap is only valid for map", name)
	}
	if visual.Type == "map" && (presentation.Basemap != "" || presentation.Roam) {
		return fmt.Errorf("visual %q map presentation.basemap and presentation.roam were replaced by geo.basemap and geo.controls", name)
	}
	if presentation.InnerRadius < 0 || presentation.InnerRadius > 1 || presentation.OuterRadius < 0 || presentation.OuterRadius > 1 || (presentation.InnerRadius > 0 && presentation.OuterRadius > 0 && presentation.InnerRadius >= presentation.OuterRadius) {
		return fmt.Errorf("visual %q has invalid presentation radii", name)
	}
	if (presentation.InnerRadius > 0 || presentation.OuterRadius > 0 || presentation.CenterLabel != "") && visual.Type != "donut" {
		return fmt.Errorf("visual %q donut presentation is only valid for donut", name)
	}
	if presentation.Rose && visual.Type != "pie" && visual.Type != "donut" {
		return fmt.Errorf("visual %q presentation.rose is only valid for pie or donut", name)
	}
	if presentation.Align != "" && (visual.Type != "funnel" || !oneOf(presentation.Align, "left", "center", "right")) {
		return fmt.Errorf("visual %q has unsupported presentation.align %q", name, presentation.Align)
	}
	if presentation.Sort != "" && (visual.Type != "funnel" || !oneOf(presentation.Sort, "ascending", "descending")) {
		return fmt.Errorf("visual %q has unsupported presentation.sort %q", name, presentation.Sort)
	}
	if presentation.Layout != "" && (!oneOf(visual.Type, "tree", "graph") || !oneOf(presentation.Layout, "standard", "circular")) {
		return fmt.Errorf("visual %q has unsupported presentation.layout %q", name, presentation.Layout)
	}
	if presentation.Focus != "" && (!oneOf(visual.Type, "graph", "sankey") || !oneOf(presentation.Focus, "none", "adjacency")) {
		return fmt.Errorf("visual %q has unsupported presentation.focus %q", name, presentation.Focus)
	}
	if presentation.InitialDepth < 0 || (presentation.InitialDepth > 0 && !oneOf(visual.Type, "tree", "treemap", "sunburst")) {
		return fmt.Errorf("visual %q has unsupported presentation.initial_depth %d", name, presentation.InitialDepth)
	}
	if presentation.NodeGap < 0 || (presentation.NodeGap > 0 && visual.Type != "sankey") {
		return fmt.Errorf("visual %q has unsupported presentation.node_gap %v", name, presentation.NodeGap)
	}
	if presentation.Curveness < 0 || presentation.Curveness > 1 || (presentation.Curveness > 0 && !oneOf(visual.Type, "graph", "sankey")) {
		return fmt.Errorf("visual %q has unsupported presentation.curveness %v", name, presentation.Curveness)
	}
	if presentation.Breadcrumb != nil && visual.Type != "treemap" {
		return fmt.Errorf("visual %q presentation.breadcrumb is only valid for treemap", name)
	}
	if presentation.Roam && !oneOf(visual.Type, "tree", "treemap", "sunburst", "graph") {
		return fmt.Errorf("visual %q presentation.roam is unsupported for type %q", name, visual.Type)
	}
	if (presentation.Minimum != nil || presentation.Maximum != nil || presentation.ProgressWidth > 0 || len(presentation.Thresholds) > 0) && visual.Type != "gauge" && visual.Type != "kpi" {
		return fmt.Errorf("visual %q threshold presentation is only valid for gauge or kpi", name)
	}
	if presentation.Minimum != nil && presentation.Maximum != nil && *presentation.Minimum >= *presentation.Maximum {
		return fmt.Errorf("visual %q presentation.minimum must be less than maximum", name)
	}
	previous := -1.0e308
	for _, threshold := range presentation.Thresholds {
		if threshold.Value < previous {
			return fmt.Errorf("visual %q presentation.thresholds must be ordered", name)
		}
		if !oneOf(threshold.Tone, "neutral", "ink", "success", "warning", "danger") {
			return fmt.Errorf("visual %q has unsupported threshold tone %q", name, threshold.Tone)
		}
		previous = threshold.Value
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (d *Dashboard) validateSelectionInteraction(sourceKind, sourceID, kind string, selection SelectionInteraction) error {
	if len(selection.Mappings) == 0 {
		return fmt.Errorf("%s %q %s requires mappings", sourceKind, sourceID, kind)
	}
	for index, mapping := range selection.Mappings {
		if mapping.Field == "" || mapping.Value == "" {
			return fmt.Errorf("%s %q %s mapping %d requires field and value", sourceKind, sourceID, kind, index)
		}
	}
	for _, target := range selection.Targets {
		if err := d.validateInteractionTarget(sourceKind, sourceID, kind, target); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dashboard) validateInteractionTarget(sourceKind, sourceID, kind, target string) error {
	if target == "" {
		return fmt.Errorf("%s %q %s has empty target", sourceKind, sourceID, kind)
	}
	if _, ok := d.Visuals[target]; !ok {
		return fmt.Errorf("%s %q %s references unknown target %q", sourceKind, sourceID, kind, target)
	}
	return nil
}

func (d *Dashboard) validatePages() error {
	seenPages := map[string]struct{}{}
	for index, page := range d.Pages {
		if page.ID == "" || page.Title == "" {
			return fmt.Errorf("page %d requires id and title", index)
		}
		page = page.WithDefaults()
		if _, exists := seenPages[page.ID]; exists {
			return fmt.Errorf("duplicate page id %q", page.ID)
		}
		seenPages[page.ID] = struct{}{}
		for _, visual := range page.Visuals {
			if visual.ID == "" || visual.Kind == "" {
				return fmt.Errorf("page %q has a visual missing id or kind", page.ID)
			}
			if err := validatePlacement(page, visual); err != nil {
				return err
			}
			switch visual.Kind {
			case "header":
				if visual.Visual != "" || visual.Binding.ID != "" {
					return fmt.Errorf("page %q header %q must not reference a visual or filter binding", page.ID, visual.ID)
				}
			case "slicer":
				if visual.Visual != "" {
					return fmt.Errorf("page %q slicer %q must not reference a visual", page.ID, visual.ID)
				}
				if visual.Binding.ID == "" || !d.bindingReferenceExists(page.ID, visual.Binding) {
					return fmt.Errorf("page %q slicer %q references unknown filter binding %s/%s", page.ID, visual.ID, visual.Binding.Scope, visual.Binding.ID)
				}
			case "visual":
				if visual.Visual == "" {
					return fmt.Errorf("page %q visual %q requires visual", page.ID, visual.ID)
				}
				if _, ok := d.Visuals[visual.Visual]; !ok {
					return fmt.Errorf("page %q references unknown visual %q", page.ID, visual.Visual)
				}
				if visual.Binding.ID != "" {
					return fmt.Errorf("page %q visual %q must not reference a filter binding", page.ID, visual.ID)
				}
			default:
				return fmt.Errorf("page %q visual %q has unsupported kind %q", page.ID, visual.ID, visual.Kind)
			}
		}
	}
	return nil
}
