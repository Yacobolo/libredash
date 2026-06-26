package report

import (
	"context"

	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
)

type QueryField struct {
	Field   string
	Alias   string
	Measure InlineMeasure
}

type InlineMeasure struct {
	Field       string
	Name        string
	Label       string
	Description string
	Expr        string
	Expression  string
	Table       string
	Grain       string
	Time        string
	Grains      []string
	Unit        string
	Format      string
}

type QueryFilter struct {
	Field    string
	Operator string
	Values   []any
	Groups   []QueryFilterGroup
}

type QueryFilterGroup struct {
	Filters []QueryFilter
}

type QuerySort struct {
	Field     string
	Direction string
}

type AggregateQuery struct {
	Table      string
	Dimensions []QueryField
	Measures   []QueryField
	Time       QueryTime
	Filters    []QueryFilter
	Sort       []QuerySort
	Limit      int
	Offset     int
}

type RowQuery struct {
	Table      string
	Dimensions []QueryField
	Measures   []QueryField
	Filters    []QueryFilter
	Sort       []QuerySort
	Limit      int
	Offset     int
}

type RawValueQuery struct {
	Table      string
	Dimensions []QueryField
	Measure    QueryField
	Filters    []QueryFilter
}

type CountQuery struct {
	Table   string
	Filters []QueryFilter
}

type QueryRow map[string]any

type QueryRows []QueryRow

type HistogramBin struct {
	Bucket int
	Count  int
	Start  float64
	End    float64
}

type DataService interface {
	Query(ctx context.Context, request AggregateQuery) (QueryRows, error)
	Rows(ctx context.Context, request RowQuery) (QueryRows, error)
	Count(ctx context.Context, request CountQuery) (int, error)
	Histogram(ctx context.Context, request RawValueQuery, binCount int) ([]HistogramBin, error)
	Distribution(ctx context.Context, request RawValueQuery, sort []QuerySort, limit int) (QueryRows, error)
}

type AnalyticsQueries interface {
	Query(ctx context.Context, request semanticquery.Request) (semanticquery.Rows, error)
	Rows(ctx context.Context, request semanticquery.RowRequest) (semanticquery.Rows, error)
	Count(ctx context.Context, request semanticquery.CountRequest) (int, error)
	Histogram(ctx context.Context, request semanticquery.RawValueRequest, binCount int) ([]semanticquery.HistogramBin, error)
	Distribution(ctx context.Context, request semanticquery.RawValueRequest, sort []semanticquery.Sort, limit int) (semanticquery.Rows, error)
}

type analyticsDataService struct {
	queries AnalyticsQueries
}

func NewAnalyticsDataService(queries AnalyticsQueries) DataService {
	return analyticsDataService{queries: queries}
}

func (s analyticsDataService) Query(ctx context.Context, request AggregateQuery) (QueryRows, error) {
	rows, err := s.queries.Query(ctx, SemanticAggregateRequest(request))
	if err != nil {
		return nil, err
	}
	return queryRows(rows), nil
}

func (s analyticsDataService) Rows(ctx context.Context, request RowQuery) (QueryRows, error) {
	rows, err := s.queries.Rows(ctx, SemanticRowRequest(request))
	if err != nil {
		return nil, err
	}
	return queryRows(rows), nil
}

func SemanticAggregateRequest(request AggregateQuery) semanticquery.Request {
	return semanticquery.Request{
		Table:      request.Table,
		Dimensions: queryFields(request.Dimensions),
		Measures:   queryFields(request.Measures),
		Time:       semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:    queryFilters(request.Filters),
		Sort:       querySorts(request.Sort),
		Limit:      request.Limit,
		Offset:     request.Offset,
	}
}

func SemanticRowRequest(request RowQuery) semanticquery.RowRequest {
	return semanticquery.RowRequest{
		Table:      request.Table,
		Dimensions: queryFields(request.Dimensions),
		Measures:   queryFields(request.Measures),
		Filters:    queryFilters(request.Filters),
		Sort:       querySorts(request.Sort),
		Limit:      request.Limit,
		Offset:     request.Offset,
	}
}

func (s analyticsDataService) Count(ctx context.Context, request CountQuery) (int, error) {
	return s.queries.Count(ctx, semanticquery.CountRequest{
		Table:   request.Table,
		Filters: queryFilters(request.Filters),
	})
}

func (s analyticsDataService) Histogram(ctx context.Context, request RawValueQuery, binCount int) ([]HistogramBin, error) {
	bins, err := s.queries.Histogram(ctx, semanticquery.RawValueRequest{
		Table:      request.Table,
		Dimensions: queryFields(request.Dimensions),
		Measure:    queryField(request.Measure),
		Filters:    queryFilters(request.Filters),
	}, binCount)
	if err != nil {
		return nil, err
	}
	result := make([]HistogramBin, len(bins))
	for i, bin := range bins {
		result[i] = HistogramBin{
			Bucket: bin.Bucket,
			Count:  bin.Count,
			Start:  bin.Start,
			End:    bin.End,
		}
	}
	return result, nil
}

func (s analyticsDataService) Distribution(ctx context.Context, request RawValueQuery, sort []QuerySort, limit int) (QueryRows, error) {
	rows, err := s.queries.Distribution(ctx, semanticquery.RawValueRequest{
		Table:      request.Table,
		Dimensions: queryFields(request.Dimensions),
		Measure:    queryField(request.Measure),
		Filters:    queryFilters(request.Filters),
	}, querySorts(sort), limit)
	if err != nil {
		return nil, err
	}
	return queryRows(rows), nil
}

func queryRows(rows semanticquery.Rows) QueryRows {
	result := make(QueryRows, 0, len(rows))
	for _, row := range rows {
		out := QueryRow{}
		for key, value := range row {
			out[key] = value
		}
		result = append(result, out)
	}
	return result
}

func queryFields(fields []QueryField) []semanticquery.Field {
	result := make([]semanticquery.Field, len(fields))
	for i, field := range fields {
		result[i] = queryField(field)
	}
	return result
}

func queryField(field QueryField) semanticquery.Field {
	return semanticquery.Field{
		Field:   field.Field,
		Alias:   field.Alias,
		Measure: queryInlineMeasure(field.Measure),
	}
}

func queryInlineMeasure(measure InlineMeasure) semanticquery.InlineMeasure {
	return semanticquery.InlineMeasure{
		Field:       measure.Field,
		Name:        measure.Name,
		Label:       measure.Label,
		Description: measure.Description,
		Expr:        measure.Expr,
		Expression:  measure.Expression,
		Table:       measure.Table,
		Grain:       measure.Grain,
		Time:        measure.Time,
		Grains:      append([]string{}, measure.Grains...),
		Unit:        measure.Unit,
		Format:      measure.Format,
	}
}

func queryFilters(filters []QueryFilter) []semanticquery.Filter {
	result := make([]semanticquery.Filter, len(filters))
	for i, filter := range filters {
		result[i] = semanticquery.Filter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   queryFilterGroups(filter.Groups),
		}
	}
	return result
}

func queryFilterGroups(groups []QueryFilterGroup) []semanticquery.FilterGroup {
	result := make([]semanticquery.FilterGroup, len(groups))
	for i, group := range groups {
		result[i] = semanticquery.FilterGroup{Filters: queryFilters(group.Filters)}
	}
	return result
}

func querySorts(sorts []QuerySort) []semanticquery.Sort {
	result := make([]semanticquery.Sort, len(sorts))
	for i, sort := range sorts {
		result[i] = semanticquery.Sort{Field: sort.Field, Direction: sort.Direction}
	}
	return result
}
