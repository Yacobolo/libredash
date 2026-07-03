package runtime

import (
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

func reportAggregateDataQuery(modelID string, request reportdef.AggregateQuery) dataquery.Query {
	return dataquery.Query{
		ModelID:  modelID,
		Kind:     dataquery.KindSemanticAggregate,
		Target:   request.Table,
		Fields:   reportFieldsToDataFields(request.Dimensions),
		Measures: reportFieldsToDataFields(request.Measures),
		Time:     dataquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:  reportFiltersToDataFilters(request.Filters),
		Sort:     reportSortToDataSort(request.Sort),
		Limit:    request.Limit,
		Offset:   request.Offset,
	}
}

func reportRowDataQuery(modelID string, request reportdef.RowQuery, includeTotal bool) dataquery.Query {
	return dataquery.Query{
		ModelID:      modelID,
		Kind:         dataquery.KindSemanticRows,
		Target:       request.Table,
		Fields:       reportFieldsToDataFields(request.Dimensions),
		Measures:     reportFieldsToDataFields(request.Measures),
		Filters:      reportFiltersToDataFilters(request.Filters),
		Sort:         reportSortToDataSort(request.Sort),
		Limit:        request.Limit,
		Offset:       request.Offset,
		IncludeTotal: includeTotal,
	}
}

func reportFieldsToDataFields(fields []reportdef.QueryField) []dataquery.Field {
	out := make([]dataquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, dataquery.Field{
			Field: field.Field,
			Alias: field.Alias,
			Measure: dataquery.InlineMeasure{
				Field:       field.Measure.Field,
				Name:        field.Measure.Name,
				Label:       field.Measure.Label,
				Description: field.Measure.Description,
				Expr:        field.Measure.Expr,
				Expression:  field.Measure.Expression,
				Table:       field.Measure.Table,
				Grain:       field.Measure.Grain,
				Time:        field.Measure.Time,
				Grains:      append([]string{}, field.Measure.Grains...),
				Unit:        field.Measure.Unit,
				Format:      field.Measure.Format,
			},
		})
	}
	return out
}

func reportFiltersToDataFilters(filters []reportdef.QueryFilter) []dataquery.Filter {
	out := make([]dataquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]dataquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, dataquery.FilterGroup{Filters: reportFiltersToDataFilters(group.Filters)})
		}
		out = append(out, dataquery.Filter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   groups,
		})
	}
	return out
}

func reportSortToDataSort(sort []reportdef.QuerySort) []dataquery.Sort {
	out := make([]dataquery.Sort, 0, len(sort))
	for _, item := range sort {
		out = append(out, dataquery.Sort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func reportRowsFromDataQuery(rows []dataquery.Row) reportdef.QueryRows {
	out := make(reportdef.QueryRows, 0, len(rows))
	for _, row := range rows {
		converted := reportdef.QueryRow{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
}
