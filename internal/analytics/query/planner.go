package query

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
)

func (p *Planner) Plan(request Request) (Plan, error) {
	return p.planAggregate(request)
}

func (p *Planner) PlanRows(request RowRequest) (Plan, error) {
	view, err := p.rowView(request)
	if err != nil {
		return Plan{}, err
	}
	masks, err := columnMaskMap(request.ColumnMasks)
	if err != nil {
		return Plan{}, err
	}
	fieldSet := []string{}
	for _, dimension := range request.Dimensions {
		field, _, err := view.ResolveDimensionRef(dimension.Field)
		if err != nil {
			return Plan{}, err
		}
		fieldSet = append(fieldSet, field)
	}
	for _, measure := range request.Measures {
		field, resolved, err := view.ResolveMeasureRef(measure.Field)
		if err != nil {
			return Plan{}, err
		}
		if resolved.Fact != view.Fact {
			return Plan{}, fmt.Errorf("measure %q is not owned by fact %q", field, view.Fact)
		}
		fieldSet = append(fieldSet, measurePhysicalFields(resolved)...)
	}
	filterFields, err := filterFieldSet(view, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	fieldSet = append(fieldSet, filterFields...)
	aliases, err := p.aliases(view, fieldSet)
	if err != nil {
		return Plan{}, err
	}
	from, err := joinSQL(p.Model, view.Fact, aliases)
	if err != nil {
		return Plan{}, err
	}
	selects := []string{}
	columns := []string{}
	columnSet := map[string]bool{}
	for _, item := range request.Dimensions {
		field, _, _ := view.ResolveDimensionRef(item.Field)
		alias, err := outputAlias(Field{Field: field, Alias: item.Alias})
		if err != nil {
			return Plan{}, err
		}
		if err := addOutputColumn(columnSet, alias); err != nil {
			return Plan{}, err
		}
		expr, err := maskedDimensionExpr(field, view.Dimensions[field], aliases, masks)
		if err != nil {
			return Plan{}, err
		}
		selects = append(selects, expr+" AS "+alias)
		columns = append(columns, alias)
	}
	for _, item := range request.Measures {
		field, _, _ := view.ResolveMeasureRef(item.Field)
		alias, err := outputAlias(Field{Field: field, Alias: item.Alias})
		if err != nil {
			return Plan{}, err
		}
		if err := addOutputColumn(columnSet, alias); err != nil {
			return Plan{}, err
		}
		expr, err := maskedRawMeasureExpr(p.Model, field, view.Measures[field], aliases, masks)
		if err != nil {
			return Plan{}, err
		}
		selects = append(selects, expr+" AS "+alias)
		columns = append(columns, alias)
	}
	if len(selects) == 0 {
		return Plan{}, fmt.Errorf("row query requires at least one selected field")
	}
	whereParts, args, err := p.whereParts(view, aliases, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	var sql strings.Builder
	sql.WriteString("SELECT ")
	sql.WriteString(strings.Join(selects, ", "))
	sql.WriteString("\nFROM ")
	sql.WriteString(from)
	sql.WriteString("\nWHERE ")
	sql.WriteString(strings.Join(whereParts, " AND "))
	if err := writeOrderLimitOffset(&sql, request.Sort, columnSet, request.Limit, request.Offset); err != nil {
		return Plan{}, err
	}
	return Plan{SQL: sql.String(), Args: args, Columns: columns}, nil
}

func (p *Planner) PlanRawValues(request RawValueRequest) (Plan, error) {
	view, err := p.rawValueView(request)
	if err != nil {
		return Plan{}, err
	}
	masks, err := columnMaskMap(request.ColumnMasks)
	if err != nil {
		return Plan{}, err
	}
	fieldSet := []string{}
	for _, dimension := range request.Dimensions {
		field, _, err := view.ResolveDimensionRef(dimension.Field)
		if err != nil {
			return Plan{}, err
		}
		fieldSet = append(fieldSet, field)
	}
	measureField, measure, err := view.ResolveMeasureRef(request.Measure.Field)
	if err != nil {
		return Plan{}, err
	}
	if measure.Fact != view.Fact {
		return Plan{}, fmt.Errorf("measure %q is not owned by fact %q", measureField, view.Fact)
	}
	if masks.matchesMeasure(measureField, measure) {
		return Plan{}, fmt.Errorf("measure %q depends on a masked field", measureField)
	}
	fieldSet = append(fieldSet, measurePhysicalFields(measure)...)
	filterFields, err := filterFieldSet(view, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	fieldSet = append(fieldSet, filterFields...)
	aliases, err := p.aliases(view, fieldSet)
	if err != nil {
		return Plan{}, err
	}
	from, err := joinSQL(p.Model, view.Fact, aliases)
	if err != nil {
		return Plan{}, err
	}
	selects := []string{}
	columns := []string{}
	columnSet := map[string]bool{}
	dimensionFields := []string{}
	for _, item := range request.Dimensions {
		field, _, _ := view.ResolveDimensionRef(item.Field)
		alias, err := outputAlias(Field{Field: field, Alias: item.Alias})
		if err != nil {
			return Plan{}, err
		}
		if err := addOutputColumn(columnSet, alias); err != nil {
			return Plan{}, err
		}
		expr, err := maskedDimensionExpr(field, view.Dimensions[field], aliases, masks)
		if err != nil {
			return Plan{}, err
		}
		selects = append(selects, expr+" AS "+alias)
		columns = append(columns, alias)
		dimensionFields = append(dimensionFields, field)
	}
	rawExpr, err := rawMeasureExpr(p.Model, measure, aliases)
	if err != nil {
		return Plan{}, err
	}
	valueAlias := request.Measure.Alias
	if valueAlias == "" {
		valueAlias = "value"
	}
	if _, err := quoteIdent(valueAlias); err != nil {
		return Plan{}, err
	}
	if err := addOutputColumn(columnSet, valueAlias); err != nil {
		return Plan{}, err
	}
	selects = append(selects, "CAST("+rawExpr+" AS DOUBLE) AS "+valueAlias)
	columns = append(columns, valueAlias)
	whereParts, args, err := p.whereParts(view, aliases, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	for _, field := range dimensionFields {
		if where := dimensionWhereExpr(view.Dimensions[field], aliases); where != "" {
			whereParts = append(whereParts, where)
		}
	}
	whereParts = append(whereParts, rawExpr+" IS NOT NULL")
	var sql strings.Builder
	sql.WriteString("SELECT ")
	sql.WriteString(strings.Join(selects, ", "))
	sql.WriteString("\nFROM ")
	sql.WriteString(from)
	sql.WriteString("\nWHERE ")
	sql.WriteString(strings.Join(whereParts, " AND "))
	if err := writeOrderLimitOffset(&sql, request.Sort, columnSet, request.Limit, 0); err != nil {
		return Plan{}, err
	}
	return Plan{SQL: sql.String(), Args: args, Columns: columns}, nil
}

func (p *Planner) PlanCount(request CountRequest) (Plan, error) {
	view, err := p.countView(request)
	if err != nil {
		return Plan{}, err
	}
	fieldSet := []string{}
	filterFields, err := filterFieldSet(view, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	fieldSet = append(fieldSet, filterFields...)
	aliases, err := p.aliases(view, fieldSet)
	if err != nil {
		return Plan{}, err
	}
	from, err := joinSQL(p.Model, view.Fact, aliases)
	if err != nil {
		return Plan{}, err
	}
	whereParts, args, err := p.whereParts(view, aliases, request.Filters)
	if err != nil {
		return Plan{}, err
	}
	sql := "SELECT COUNT(*) AS value\nFROM " + from + "\nWHERE " + strings.Join(whereParts, " AND ")
	return Plan{SQL: sql, Args: args, Columns: []string{"value"}}, nil
}

func (p *Planner) whereParts(view *queryView, aliases map[string]tableAlias, filters []Filter) ([]string, []any, error) {
	whereParts := []string{"1 = 1"}
	args := []any{}
	for _, filter := range filters {
		part, partArgs, err := p.filterPart(view, aliases, filter)
		if err != nil {
			return nil, nil, err
		}
		if part != "" {
			whereParts = append(whereParts, part)
			args = append(args, partArgs...)
		}
	}
	return whereParts, args, nil
}

func (p *Planner) filterPart(view *queryView, aliases map[string]tableAlias, filter Filter) (string, []any, error) {
	if len(filter.Groups) > 0 {
		parts := []string{}
		args := []any{}
		for _, group := range filter.Groups {
			groupParts := []string{}
			for _, child := range group.Filters {
				part, partArgs, err := p.filterPart(view, aliases, child)
				if err != nil {
					return "", nil, err
				}
				if part == "" {
					continue
				}
				groupParts = append(groupParts, part)
				args = append(args, partArgs...)
			}
			if len(groupParts) > 0 {
				parts = append(parts, "("+strings.Join(groupParts, " AND ")+")")
			}
		}
		if len(parts) == 0 {
			return "", nil, nil
		}
		return "(" + strings.Join(parts, " OR ") + ")", args, nil
	}
	if filter.Field == "" {
		return "", nil, nil
	}
	_, dimension, _ := view.ResolveDimensionRef(filter.Field)
	expr := dimensionExpr(dimension, aliases)
	return filterSQL(expr, filter)
}

type columnMaskSet map[string]string

func columnMaskMap(masks []ColumnMask) (columnMaskSet, error) {
	out := columnMaskSet{}
	for _, mask := range masks {
		field := strings.ToLower(strings.TrimSpace(mask.Field))
		if field == "" {
			continue
		}
		normalizedMask := strings.ToLower(strings.TrimSpace(mask.Mask))
		if normalizedMask == "" {
			normalizedMask = "null"
		}
		if _, err := maskSQLExpr(normalizedMask); err != nil {
			return nil, err
		}
		out[field] = normalizedMask
	}
	return out, nil
}

func (m columnMaskSet) matchesDimension(ref string, dimension semanticmodel.MetricDimension) bool {
	if len(m) == 0 {
		return false
	}
	if _, ok := m[strings.ToLower(strings.TrimSpace(ref))]; ok {
		return true
	}
	return false
}

func (m columnMaskSet) matchesMeasure(ref string, measure ResolvedMeasure) bool {
	if len(m) == 0 {
		return false
	}
	for _, key := range []string{ref, measure.Field} {
		if _, ok := m[strings.ToLower(strings.TrimSpace(key))]; ok {
			return true
		}
	}
	for _, dependency := range measurePhysicalFields(measure) {
		if _, ok := m[strings.ToLower(strings.TrimSpace(dependency))]; ok {
			return true
		}
	}
	return false
}

func maskedDimensionExpr(ref string, dimension semanticmodel.MetricDimension, aliases map[string]tableAlias, masks columnMaskSet) (string, error) {
	mask, ok := masks[strings.ToLower(strings.TrimSpace(ref))]
	if !ok {
		return dimensionExpr(dimension, aliases), nil
	}
	return maskSQLExpr(mask)
}

func maskedRawMeasureExpr(model *semanticmodel.Model, ref string, measure ResolvedMeasure, aliases map[string]tableAlias, masks columnMaskSet) (string, error) {
	if mask, ok := masks[strings.ToLower(strings.TrimSpace(ref))]; ok {
		return maskSQLExpr(mask)
	}
	for _, dependency := range measurePhysicalFields(measure) {
		if mask, ok := masks[strings.ToLower(strings.TrimSpace(dependency))]; ok {
			return maskSQLExpr(mask)
		}
	}
	return rawMeasureExpr(model, measure, aliases)
}

func measurePhysicalFields(measure ResolvedMeasure) []string {
	fields := []string{}
	if measure.InputField != "" {
		fields = append(fields, measure.InputField)
	}
	if measure.InputExpr != "" {
		if measure.InputExpression != nil {
			fields = append(fields, measure.InputExpression.References()...)
		} else if expression, err := semanticmodel.ParseExpression(measure.InputExpr); err == nil {
			fields = append(fields, expression.References()...)
		}
	}
	for _, filter := range measure.Filters {
		if filter.Field != "" {
			fields = append(fields, filter.Field)
		}
	}
	return fields
}

func maskSQLExpr(mask string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mask)) {
	case "", "null":
		return "NULL", nil
	case "redact", "redacted":
		return "'REDACTED'", nil
	case "zero":
		return "0", nil
	default:
		return "", fmt.Errorf("unsupported column mask %q", mask)
	}
}

func filterFieldSet(view *queryView, filters []Filter) ([]string, error) {
	fields := []string{}
	for _, filter := range filters {
		items, err := filterFields(view, filter)
		if err != nil {
			return nil, err
		}
		fields = append(fields, items...)
	}
	return fields, nil
}

func filterFields(view *queryView, filter Filter) ([]string, error) {
	fields := []string{}
	if filter.Field != "" {
		field, _, err := view.ResolveDimensionRef(filter.Field)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	for _, group := range filter.Groups {
		items, err := filterFieldSet(view, group.Filters)
		if err != nil {
			return nil, err
		}
		fields = append(fields, items...)
	}
	return fields, nil
}

func allowedTimeGrain(grain string) bool {
	switch grain {
	case "day", "week", "month", "quarter", "year":
		return true
	default:
		return false
	}
}

func fieldAlias(field string) string {
	if field == "value" || field == "" {
		return field
	}
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func outputAlias(field Field) (string, error) {
	if field.Alias != "" {
		if _, err := quoteIdent(field.Alias); err != nil {
			return "", err
		}
		return field.Alias, nil
	}
	alias := fieldAlias(field.Field)
	if _, err := quoteIdent(alias); err != nil {
		return "", err
	}
	return alias, nil
}

func addOutputColumn(columns map[string]bool, alias string) error {
	if columns[alias] {
		return fmt.Errorf("duplicate output alias %q", alias)
	}
	columns[alias] = true
	return nil
}

func sortSQL(sorts []Sort, columns map[string]bool) ([]string, error) {
	parts := make([]string, 0, len(sorts))
	for _, sort := range sorts {
		field, err := quoteIdent(sort.Field)
		if err != nil {
			return nil, err
		}
		if !columns[field] {
			return nil, fmt.Errorf("sort field %q is not a selected output alias", sort.Field)
		}
		direction := "ASC"
		switch {
		case sort.Direction == "" || strings.EqualFold(sort.Direction, "asc"):
			direction = "ASC"
		case strings.EqualFold(sort.Direction, "desc"):
			direction = "DESC"
		default:
			return nil, fmt.Errorf("unsupported sort direction %q", sort.Direction)
		}
		parts = append(parts, field+" "+direction)
	}
	return parts, nil
}

func writeOrderLimitOffset(sql *strings.Builder, sorts []Sort, columns map[string]bool, limit, offset int) error {
	if len(sorts) > 0 {
		parts, err := sortSQL(sorts, columns)
		if err != nil {
			return err
		}
		sql.WriteString("\nORDER BY ")
		sql.WriteString(strings.Join(parts, ", "))
	}
	return writeLimitOffset(sql, limit, offset)
}

func writeLimitOffset(sql *strings.Builder, limit, offset int) error {
	if limit > 0 {
		sql.WriteString(fmt.Sprintf("\nLIMIT %d", limit))
	}
	if offset > 0 {
		if limit <= 0 {
			return fmt.Errorf("offset requires limit")
		}
		sql.WriteString(fmt.Sprintf("\nOFFSET %d", offset))
	}
	return nil
}
