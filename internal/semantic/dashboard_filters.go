package semantic

import "fmt"

func (targets InteractionTargets) IsEmpty() bool {
	return len(targets.Visuals) == 0 && len(targets.Tables) == 0
}

func (targets InteractionTargets) Contains(kind, id string) bool {
	switch kind {
	case "visual":
		return containsString(targets.Visuals, id)
	case "table":
		return containsString(targets.Tables, id)
	default:
		return false
	}
}

func (d *Dashboard) FilterAppliesToTarget(model *Model, filter FilterDefinition, targetKind, targetID string) (bool, error) {
	targeted := !filter.Targets.IsEmpty()
	if targeted && !filter.Targets.Contains(targetKind, targetID) {
		return false, nil
	}
	baseTable, err := d.TargetBaseTable(model, targetKind, targetID)
	if err != nil {
		return false, err
	}
	if err := model.CanReachField(baseTable, filter.Dimension); err != nil {
		if targeted {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (d *Dashboard) TargetBaseTable(model *Model, targetKind, targetID string) (string, error) {
	switch targetKind {
	case "visual":
		visual, ok := d.Visuals[targetID]
		if !ok {
			return "", fmt.Errorf("unknown target visual %q", targetID)
		}
		return visualQueryBaseTable(model, visual.Query)
	case "table":
		table, ok := d.Tables[targetID]
		if !ok {
			return "", fmt.Errorf("unknown target table %q", targetID)
		}
		return tableQueryBaseTable(model, table)
	default:
		return "", fmt.Errorf("unknown target kind %q", targetKind)
	}
}

func visualQueryBaseTable(model *Model, query VisualQuery) (string, error) {
	base, err := measureRefsBaseTable(model, query.Measures)
	if err != nil {
		return "", err
	}
	if base != "" {
		return base, nil
	}
	if len(query.Dimensions) > 0 {
		dimension, err := model.ResolveDimension(query.Dimensions[0].Field)
		if err != nil {
			return "", err
		}
		return dimension.Table, nil
	}
	if !query.Series.IsZero() {
		dimension, err := model.ResolveDimension(query.Series.Field)
		if err != nil {
			return "", err
		}
		return dimension.Table, nil
	}
	return "", fmt.Errorf("query requires a base table")
}

func tableQueryBaseTable(model *Model, table TableVisual) (string, error) {
	if table.Query.Table != "" {
		return table.Query.Table, nil
	}
	columns := table.DataColumns
	if len(columns) == 0 {
		columns = table.Query.Columns
	}
	for _, column := range columns {
		if measure, err := model.ResolveMeasure(column.Field); err == nil {
			return measure.Table, nil
		}
		if dimension, err := model.ResolveDimension(column.Field); err == nil {
			return dimension.Table, nil
		}
	}
	base, err := measureRefsBaseTable(model, table.Query.Measures)
	if err != nil {
		return "", err
	}
	if base != "" {
		return base, nil
	}
	if len(table.Query.Rows) > 0 {
		dimension, err := model.ResolveDimension(table.Query.Rows[0].Field)
		if err != nil {
			return "", err
		}
		return dimension.Table, nil
	}
	return "", fmt.Errorf("query requires a base table")
}

func measureRefsBaseTable(model *Model, measures []FieldRef) (string, error) {
	base := ""
	grain := ""
	for _, ref := range measures {
		measure, err := metricMeasureForRef(model, ref)
		if err != nil {
			return "", err
		}
		if measure.Table == "" {
			return "", fmt.Errorf("measure %q has no base table", ref.Field)
		}
		if base == "" {
			base = measure.Table
			grain = measure.Grain
			continue
		}
		if measure.Table != base || (grain != "" && measure.Grain != "" && measure.Grain != grain) {
			return "", fmt.Errorf("cross-fact measures are not supported")
		}
		if grain == "" {
			grain = measure.Grain
		}
	}
	return base, nil
}

func metricMeasureForRef(model *Model, ref FieldRef) (MetricMeasure, error) {
	if ref.Measure.Expression != "" || ref.Measure.Expr != "" {
		measure := ref.Measure
		measure.Field = defaultString(measure.Field, ref.Field)
		measure.Name = defaultString(measure.Name, ref.Field)
		return measure, nil
	}
	return model.ResolveMeasure(ref.Field)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
