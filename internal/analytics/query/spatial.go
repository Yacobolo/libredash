package query

import (
	"fmt"
	"math"
	"strings"
)

const SpatialTotalColumn = "__spatial_total"

// PlanSpatial wraps a complete governed semantic aggregate in a bounded
// viewport projection. The inner plan deliberately has no LIMIT; aggregation
// and the feature cap are applied only by the outer spatial query.
func (p *Planner) PlanSpatial(request SpatialRequest) (Plan, error) {
	if err := validateSpatialRequest(request); err != nil {
		return Plan{}, err
	}
	latitude, err := outputAlias(request.Latitude)
	if err != nil {
		return Plan{}, err
	}
	longitude, err := outputAlias(request.Longitude)
	if err != nil {
		return Plan{}, err
	}
	filters := append([]Filter{}, request.Filters...)
	filters = append(filters, spatialBoundsFilters(request)...)
	inner, err := p.Plan(Request{
		Table: request.Table, Dimensions: request.Dimensions, Measures: request.Measures,
		Time: request.Time, Filters: filters, ColumnMasks: request.ColumnMasks,
	})
	if err != nil {
		return Plan{}, err
	}
	columns := append([]string{}, inner.Columns...)
	if !containsOutputColumn(columns, latitude) || !containsOutputColumn(columns, longitude) {
		return Plan{}, fmt.Errorf("spatial coordinates must be selected output aliases")
	}
	columns = append(columns, SpatialTotalColumn)

	if request.Precision == SpatialPrecisionRaw {
		if len(request.Sort) == 0 {
			return Plan{}, fmt.Errorf("raw spatial query requires deterministic sorting")
		}
		columnSet := make(map[string]bool, len(inner.Columns))
		for _, column := range inner.Columns {
			columnSet[column] = true
		}
		var sql strings.Builder
		sql.WriteString("WITH governed AS (\n")
		sql.WriteString(inner.SQL)
		sql.WriteString("\n)\nSELECT ")
		sql.WriteString(strings.Join(inner.Columns, ", "))
		sql.WriteString(", COUNT(*) OVER () AS ")
		sql.WriteString(SpatialTotalColumn)
		sql.WriteString("\nFROM governed")
		if err := writeOrderLimitOffset(&sql, request.Sort, columnSet, request.FeatureCap, 0); err != nil {
			return Plan{}, err
		}
		return Plan{SQL: sql.String(), Args: inner.Args, Columns: columns, Mode: "spatial_raw", Facts: inner.Facts, PhysicalDependencies: inner.PhysicalDependencies, RelationshipPaths: inner.RelationshipPaths}, nil
	}

	longitudeSpan := request.East - request.West
	if longitudeSpan <= 0 {
		longitudeSpan += 360
	}
	latitudeSpan := math.Max(request.North-request.South, 0.000001)
	gridColumns := max(1, min(request.Width/48, int(math.Sqrt(float64(request.FeatureCap)*math.Max(float64(request.Width), 1)/math.Max(float64(request.Height), 1)))))
	gridRows := max(1, min(request.Height/48, request.FeatureCap/gridColumns))
	west, south := spatialNumber(request.West), spatialNumber(request.South)
	lonSpan, latSpan := spatialNumber(longitudeSpan), spatialNumber(latitudeSpan)
	longitudeOffset := fmt.Sprintf("(CASE WHEN %s < %s THEN %s + 360 ELSE %s END - %s)", longitude, west, longitude, longitude, west)
	x := fmt.Sprintf("LEAST(%d, GREATEST(0, FLOOR(%s / %s * %d)))", gridColumns-1, longitudeOffset, lonSpan, gridColumns)
	y := fmt.Sprintf("LEAST(%d, GREATEST(0, FLOOR((%s - %s) / %s * %d)))", gridRows-1, latitude, south, latSpan, gridRows)

	measureAggregates := map[string]string{}
	for _, field := range request.Measures {
		alias, err := outputAlias(field)
		if err != nil {
			return Plan{}, err
		}
		measure, err := p.Model.ResolveMeasure(field.Field)
		if err != nil {
			return Plan{}, err
		}
		switch measure.Aggregation {
		case "count", "sum":
			measureAggregates[alias] = "SUM"
		case "min":
			measureAggregates[alias] = "MIN"
		case "max":
			measureAggregates[alias] = "MAX"
		default:
			return Plan{}, fmt.Errorf("measure %q aggregation %q cannot be re-aggregated spatially", field.Field, measure.Aggregation)
		}
	}
	selects := make([]string, 0, len(inner.Columns)+1)
	for _, column := range inner.Columns {
		expression := "MIN(" + column + ")"
		switch column {
		case latitude, longitude:
			expression = "AVG(" + column + ")"
		default:
			if aggregate := measureAggregates[column]; aggregate != "" {
				expression = aggregate + "(" + column + ")"
			}
		}
		selects = append(selects, expression+" AS "+column)
	}
	selects = append(selects, "SUM(COUNT(*)) OVER () AS "+SpatialTotalColumn)

	var sql strings.Builder
	sql.WriteString("WITH governed AS (\n")
	sql.WriteString(inner.SQL)
	sql.WriteString("\n),\nbucketed AS (\nSELECT *, ")
	sql.WriteString(x)
	sql.WriteString(" AS __spatial_x, ")
	sql.WriteString(y)
	sql.WriteString(" AS __spatial_y\nFROM governed\n)\nSELECT ")
	sql.WriteString(strings.Join(selects, ", "))
	sql.WriteString("\nFROM bucketed\nGROUP BY __spatial_x, __spatial_y\nORDER BY __spatial_y, __spatial_x")
	if err := writeLimitOffset(&sql, request.FeatureCap, 0); err != nil {
		return Plan{}, err
	}
	return Plan{SQL: sql.String(), Args: inner.Args, Columns: columns, Mode: "spatial_aggregated", Facts: inner.Facts, PhysicalDependencies: inner.PhysicalDependencies, RelationshipPaths: inner.RelationshipPaths}, nil
}

func validateSpatialRequest(request SpatialRequest) error {
	values := []float64{request.West, request.South, request.East, request.North}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("spatial bounds must be finite")
		}
	}
	if request.Table == "" || request.Latitude.Field == "" || request.Longitude.Field == "" {
		return fmt.Errorf("spatial query requires table and coordinate fields")
	}
	if request.West < -180 || request.West > 180 || request.East < -180 || request.East > 180 || request.South < -90 || request.North > 90 || request.South > request.North {
		return fmt.Errorf("invalid spatial bounds")
	}
	if request.Width <= 0 || request.Height <= 0 || request.FeatureCap <= 0 {
		return fmt.Errorf("spatial query requires positive viewport dimensions and feature cap")
	}
	if request.Precision != SpatialPrecisionRaw && request.Precision != SpatialPrecisionAggregated {
		return fmt.Errorf("unsupported spatial precision %q", request.Precision)
	}
	return nil
}

func spatialBoundsFilters(request SpatialRequest) []Filter {
	filters := []Filter{
		{Field: request.Latitude.Field, Operator: "greater_than_or_equal", Values: []any{request.South}},
		{Field: request.Latitude.Field, Operator: "less_than", Values: []any{math.Nextafter(request.North, math.Inf(1))}},
	}
	if request.West <= request.East {
		return append(filters,
			Filter{Field: request.Longitude.Field, Operator: "greater_than_or_equal", Values: []any{request.West}},
			Filter{Field: request.Longitude.Field, Operator: "less_than", Values: []any{math.Nextafter(request.East, math.Inf(1))}},
		)
	}
	return append(filters, Filter{Groups: []FilterGroup{
		{Filters: []Filter{{Field: request.Longitude.Field, Operator: "greater_than_or_equal", Values: []any{request.West}}}},
		{Filters: []Filter{{Field: request.Longitude.Field, Operator: "less_than", Values: []any{math.Nextafter(request.East, math.Inf(1))}}}},
	}})
}

func containsOutputColumn(columns []string, target string) bool {
	for _, column := range columns {
		if column == target {
			return true
		}
	}
	return false
}

func spatialNumber(value float64) string {
	return fmt.Sprintf("%.17g", value)
}
