package semantic

import (
	"fmt"
	"strings"
)

func validateVisualQueryShape(name string, visual Visual) error {
	if visual.KindOrDefault() == "kpi" {
		if visual.ShapeOrDefault() != "single_value" {
			return fmt.Errorf("visual %q kind kpi requires shape single_value", name)
		}
		if len(visual.Query.Measures) != 1 {
			return fmt.Errorf("visual %q kind kpi requires exactly one query measure", name)
		}
		if len(visual.Query.Dimensions) != 0 {
			return fmt.Errorf("visual %q kind kpi does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q kind kpi does not support series", name)
		}
		return nil
	}
	shape := visual.ShapeOrDefault()
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
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_value requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_value does not support series", name)
		}
	case "category_series_value":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_series_value requires exactly one query dimension", name)
		}
		if visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_series_value requires query series", name)
		}
	case "category_multi_measure":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_multi_measure requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_multi_measure does not support series", name)
		}
	case "category_delta":
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape category_delta requires exactly one query dimension", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape category_delta does not support series", name)
		}
	case "binned_measure":
		if len(visual.Query.Dimensions) != 0 {
			return fmt.Errorf("visual %q shape binned_measure does not support query dimensions", name)
		}
		if !visual.Query.Series.IsZero() {
			return fmt.Errorf("visual %q shape binned_measure does not support series", name)
		}
	case "hierarchy":
		if len(visual.Query.Dimensions) == 0 {
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
		if len(visual.Query.Dimensions) != 1 {
			return fmt.Errorf("visual %q shape geo requires exactly one query dimension", name)
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

func validateRendererOptions(name string, options map[string]any) error {
	for renderer, value := range options {
		if !supportsRenderer(renderer) {
			return fmt.Errorf("visual %q has renderer_options for unsupported renderer %q", name, renderer)
		}
		option, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("visual %q renderer_options.%s must be an object", name, renderer)
		}
		if err := validateSafeRendererOption(name, "renderer_options."+renderer, option); err != nil {
			return err
		}
	}
	return nil
}

func validateSafeRendererOption(name, path string, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			nextPath := path + "." + key
			if key == "renderItem" {
				return fmt.Errorf("visual %q has unsafe renderer option %q", name, nextPath)
			}
			if err := validateSafeRendererOption(name, nextPath, item); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := validateSafeRendererOption(name, fmt.Sprintf("%s[%d]", path, index), item); err != nil {
				return err
			}
		}
	case string:
		if strings.Contains(typed, "function(") || strings.Contains(typed, "=>") {
			return fmt.Errorf("visual %q has unsafe renderer option %q", name, path)
		}
	}
	return nil
}
