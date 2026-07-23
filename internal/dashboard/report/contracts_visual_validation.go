package report

import (
	"fmt"
	"strings"
)

func validateVisualQueryShape(name string, visual Visual) error {
	dimensionCount := len(visual.Query.Dimensions)
	if visual.Query.Time.Field != "" {
		dimensionCount++
	}
	if visual.KindOrDefault() == "kpi" {
		if visual.ResultShape() != "single_value" {
			return fmt.Errorf("visual %q kind kpi requires shape single_value", name)
		}
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q kind kpi requires exactly one query measure", name)
		}
		if dimensionCount != 0 {
			return fmt.Errorf("visual %q kind kpi does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q kind kpi does not support series", name)
		}
		return nil
	}
	shape := visual.ResultShape()
	if (shape == "binned_measure" || shape == "distribution") && strings.TrimSpace(visual.Query.Table) == "" {
		return fmt.Errorf("visual %q shape %s requires query.table", name, shape)
	}
	switch shape {
	case "ohlc":
		if len(visual.Query.Measures) != 4 {
			return fmt.Errorf("visual %q shape ohlc requires exactly four query measures", name)
		}
	case "category_multi_measure":
		if len(visual.Query.Measures) < 2 {
			return fmt.Errorf("visual %q shape category_multi_measure requires at least two query measures", name)
		}
	default:
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q requires exactly one query measure", name)
		}
	}
	if len(visual.Query.Measures) == 0 {
		return fmt.Errorf("visual %q requires exactly one query measure", name)
	}
	switch shape {
	case "category_value":
		if dimensionCount != 1 {
			return fmt.Errorf("visual %q shape category_value requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_value does not support series", name)
		}
	case "category_series_value":
		if dimensionCount != 1 {
			return fmt.Errorf("visual %q shape category_series_value requires exactly one query dimension", name)
		}
		if visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_series_value requires query series", name)
		}
	case "category_multi_measure":
		if dimensionCount != 1 {
			return fmt.Errorf("visual %q shape category_multi_measure requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_multi_measure does not support series", name)
		}
	case "category_delta":
		if dimensionCount != 1 {
			return fmt.Errorf("visual %q shape category_delta requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_delta does not support series", name)
		}
	case "binned_measure":
		if dimensionCount != 0 {
			return fmt.Errorf("visual %q shape binned_measure does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape binned_measure does not support series", name)
		}
	case "hierarchy":
		if dimensionCount == 0 {
			return fmt.Errorf("visual %q shape hierarchy requires at least one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape hierarchy does not support series", name)
		}
	case "single_value":
		if len(visual.Query.Dimensions) > 1 {
			return fmt.Errorf("visual %q shape single_value supports at most one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape single_value does not support series", name)
		}
	case "matrix":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape matrix requires exactly two query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape matrix does not support series", name)
		}
	case "graph":
		if len(visual.Query.Dimensions) != 2 {
			return fmt.Errorf("visual %q shape graph requires exactly two query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape graph does not support series", name)
		}
	case "geo":
		if len(visual.Query.Dimensions) == 0 {
			return fmt.Errorf("visual %q shape geo requires query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape geo does not support series", name)
		}
	case "ohlc":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape ohlc requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape ohlc does not support series", name)
		}
	case "distribution":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape distribution requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape distribution does not support series", name)
		}
	}
	return nil
}

func ValidateVisualPointSelectionMappingKeys(name string, visual Visual) error {
	if !supportsPointSelection(visual) {
		return fmt.Errorf("visual %q type %q shape %q does not support point_selection", name, visual.Type, visual.ResultShape())
	}
	if visual.ResultShape() == "geo" {
		return validateGeographicPointSelectionMappingKeys(name, visual)
	}
	keys := visualPayloadKeys(visual)
	for index, mapping := range visual.Interaction.PointSelection.Mappings {
		if !keys.Contains(mapping.Value) {
			return fmt.Errorf("visual %q interaction mapping %d references unknown value key %q for shape %q", name, index, mapping.Value, visual.ResultShape())
		}
		if mapping.Label != "" && !keys.Contains(mapping.Label) {
			return fmt.Errorf("visual %q interaction mapping %d references unknown label key %q for shape %q", name, index, mapping.Label, visual.ResultShape())
		}
	}
	return nil
}

func validateGeographicPointSelectionMappingKeys(name string, visual Visual) error {
	selectable := false
	for _, layer := range visual.Geo.Layers {
		if layer.Kind == "point" || layer.Kind == "choropleth" {
			selectable = true
			break
		}
	}
	if !selectable {
		return fmt.Errorf("visual %q geographic point_selection requires at least one point or choropleth layer", name)
	}

	stableAliases := payloadKeySet{}
	allAliases := payloadKeySet{}
	add := func(keys payloadKeySet, field, alias string) {
		if field != "" {
			keys[defaultString(alias, fieldRefAlias(field))] = struct{}{}
		}
	}
	for _, field := range visual.Query.Dimensions {
		add(stableAliases, field.Field, field.Alias)
		add(allAliases, field.Field, field.Alias)
	}
	add(stableAliases, visual.Query.Time.Field, visual.Query.Time.Alias)
	add(allAliases, visual.Query.Time.Field, visual.Query.Time.Alias)
	for _, field := range visual.Query.Measures {
		add(allAliases, field.Field, field.Alias)
	}
	for index, mapping := range visual.Interaction.PointSelection.Mappings {
		if !allAliases.Contains(mapping.Value) {
			return fmt.Errorf("visual %q interaction mapping %d references unknown value query alias %q for shape %q", name, index, mapping.Value, visual.ResultShape())
		}
		if !stableAliases.Contains(mapping.Value) {
			return fmt.Errorf("visual %q interaction mapping %d value query alias %q must reference a dimension or time field", name, index, mapping.Value)
		}
		if mapping.Label != "" && !allAliases.Contains(mapping.Label) {
			return fmt.Errorf("visual %q interaction mapping %d references unknown label query alias %q for shape %q", name, index, mapping.Label, visual.ResultShape())
		}
	}
	return nil
}

func validateSpatialSelectionInteraction(name string, visual Visual) error {
	selection := visual.Interaction.SpatialSelection
	if len(selection.Gestures) == 0 {
		return fmt.Errorf("visual %q spatial_selection requires gestures", name)
	}
	seen := map[string]struct{}{}
	for _, gesture := range selection.Gestures {
		if gesture != "box" && gesture != "lasso" && gesture != "radius" {
			return fmt.Errorf("visual %q spatial_selection has unsupported gesture %q", name, gesture)
		}
		if _, ok := seen[gesture]; ok {
			return fmt.Errorf("visual %q spatial_selection has duplicate gesture %q", name, gesture)
		}
		seen[gesture] = struct{}{}
	}
	if len(selection.Targets) == 0 {
		return fmt.Errorf("visual %q spatial_selection requires targets", name)
	}
	if selection.Latitude.Source == "" || selection.Latitude.Field == "" || selection.Longitude.Source == "" || selection.Longitude.Field == "" {
		return fmt.Errorf("visual %q spatial_selection latitude and longitude require source and field", name)
	}
	if selection.Latitude.Field == selection.Longitude.Field && selection.Latitude.Fact == selection.Longitude.Fact {
		return fmt.Errorf("visual %q spatial_selection latitude and longitude target fields must differ", name)
	}
	stableAliases := payloadKeySet{}
	for _, field := range visual.Query.Dimensions {
		stableAliases[defaultString(field.Alias, fieldRefAlias(field.Field))] = struct{}{}
	}
	if visual.Query.Time.Field != "" {
		stableAliases[defaultString(visual.Query.Time.Alias, fieldRefAlias(visual.Query.Time.Field))] = struct{}{}
	}
	for axis, mapping := range map[string]SpatialSelectionMapping{"latitude": selection.Latitude, "longitude": selection.Longitude} {
		if !stableAliases.Contains(mapping.Source) {
			return fmt.Errorf("visual %q spatial_selection %s references unknown stable query alias %q", name, axis, mapping.Source)
		}
		if strings.Contains(mapping.Field, ".") && mapping.Fact == "" {
			return fmt.Errorf("visual %q spatial_selection %s physical field %q requires fact", name, axis, mapping.Field)
		}
		if !strings.Contains(mapping.Field, ".") && mapping.Fact != "" {
			return fmt.Errorf("visual %q spatial_selection %s semantic field %q must not specify fact", name, axis, mapping.Field)
		}
	}
	coordinateLayer := false
	for _, layer := range visual.Geo.Layers {
		if layer.Latitude == selection.Latitude.Source && layer.Longitude == selection.Longitude.Source && oneOf(layer.Kind, "point", "heat", "density", "path") {
			coordinateLayer = true
			break
		}
	}
	if !coordinateLayer {
		return fmt.Errorf("visual %q spatial_selection source coordinates must match one coordinate layer", name)
	}
	return nil
}

func supportsPointSelection(visual Visual) bool {
	switch visual.Type {
	case "radar":
		return false
	}
	return true
}

type payloadKeySet map[string]struct{}

func (keys payloadKeySet) Contains(key string) bool {
	_, ok := keys[key]
	return ok
}

func visualPayloadKeys(visual Visual) payloadKeySet {
	switch visual.ResultShape() {
	case "category_series_value", "category_multi_measure":
		return payloadKeys("label", "series", "value", "selected")
	case "category_delta":
		return payloadKeys("label", "value", "start", "end", "positive", "selected")
	case "binned_measure":
		return payloadKeys("label", "binStart", "binEnd", "value")
	case "hierarchy":
		keys := payloadKeys("node", "parent", "value")
		for _, field := range visual.Query.Dimensions {
			keys[defaultString(field.Alias, fieldRefAlias(field.Field))] = struct{}{}
		}
		if visual.Query.Time.Field != "" {
			keys[defaultString(visual.Query.Time.Alias, fieldRefAlias(visual.Query.Time.Field))] = struct{}{}
		}
		return keys
	case "single_value":
		return payloadKeys("label", "value", "series", "selected")
	case "matrix":
		return payloadKeys("row", "column", "value", "selected")
	case "graph":
		return payloadKeys("source", "target", "value")
	case "geo":
		return payloadKeys("name", "value", "selected")
	case "ohlc":
		return payloadKeys("label", "open", "close", "low", "high")
	case "distribution":
		return payloadKeys("label", "min", "q1", "median", "q3", "max")
	default:
		return payloadKeys("label", "value", "selected")
	}
}

func payloadKeys(values ...string) payloadKeySet {
	keys := make(payloadKeySet, len(values))
	for _, value := range values {
		keys[value] = struct{}{}
	}
	return keys
}
