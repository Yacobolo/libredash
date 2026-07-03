package materialize

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	semanticquery "github.com/Yacobolo/libredash/internal/analytics/query"
	"github.com/Yacobolo/libredash/internal/dataquery"
)

type RuntimeConfig struct {
	ModelID string
	Model   *semanticmodel.Model
	DataDir string
	DBDir   string

	Database Database
	Sources  SourceRegistrar
	Resolver SourcePathResolver
}

type ModelTableQuery struct {
	Table   string
	Columns []string
	Sort    []semanticquery.Sort
	Limit   int
	Offset  int
}

type Runtime struct {
	modelID     string
	model       *semanticmodel.Model
	dataDir     string
	db          Database
	sources     SourceRegistrar
	queries     *semanticquery.Service
	lastRefresh time.Time
}

type Database interface {
	Executor
	semanticquery.Executor
	Close() error
	Path() string
}

type schemaDiscoverer interface {
	DiscoverSchemas(context.Context, *semanticmodel.Model) error
}

func OpenRuntime(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	if config.Model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}
	if config.Database == nil {
		return nil, fmt.Errorf("materialization database is required")
	}
	if config.Sources == nil {
		return nil, fmt.Errorf("source registrar is required")
	}
	resolver := config.Resolver
	if resolver == nil {
		resolver = defaultSourcePathResolver{}
	}
	if err := ValidateFilesWithResolver(config.Model, config.DataDir, resolver); err != nil {
		return nil, err
	}
	runtime := &Runtime{
		modelID: config.ModelID,
		model:   config.Model,
		dataDir: config.DataDir,
		db:      config.Database,
		sources: config.Sources,
		queries: semanticquery.NewService(semanticquery.NewPlanner(config.Model), config.Database),
	}
	if err := runtime.Refresh(ctx); err != nil {
		config.Database.Close()
		return nil, err
	}
	return runtime, nil
}

func DatabasePath(dbDir, modelID string) string {
	if path := os.Getenv("LIBREDASH_DUCKDB_PATH"); path != "" {
		return path
	}
	return filepath.Join(dbDir, "libredash-"+modelID+".duckdb")
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Runtime) Refresh(ctx context.Context) error {
	lastRefresh, err := Refresh(ctx, r.db, r.sources, r.model)
	if err != nil {
		return err
	}
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) RefreshModelTables(ctx context.Context, tableNames []string) error {
	lastRefresh, err := RefreshModelTables(ctx, r.db, r.sources, r.model, tableNames)
	if err != nil {
		return err
	}
	if discoverer, ok := r.db.(schemaDiscoverer); ok {
		if err := discoverer.DiscoverSchemas(ctx, r.model); err != nil {
			return err
		}
	}
	r.lastRefresh = lastRefresh
	return nil
}

func (r *Runtime) Queries() *semanticquery.Service {
	if r == nil {
		return nil
	}
	return r.queries
}

func (r *Runtime) ExecuteDataQuery(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	if r == nil || r.db == nil {
		return dataquery.Result{}, fmt.Errorf("materialization runtime is not initialized")
	}
	if request.ModelID == "" {
		request.ModelID = r.modelID
	}
	if r.modelID != "" && request.ModelID != "" && request.ModelID != r.modelID {
		return dataquery.Result{}, fmt.Errorf("semantic model %q is not available in runtime for %q", request.ModelID, r.modelID)
	}
	if err := request.Validate(); err != nil {
		return dataquery.Result{}, err
	}
	switch request.Kind {
	case dataquery.KindSemanticAggregate:
		return r.executeSemanticAggregate(ctx, request)
	case dataquery.KindSemanticRows:
		return r.executeSemanticRows(ctx, request)
	case dataquery.KindModelTableRows:
		return r.executeModelTableRows(ctx, request)
	case dataquery.KindSourceRows:
		return r.executeSourceRows(ctx, request)
	case dataquery.KindSemanticHistogram:
		return r.executeSemanticHistogram(ctx, request)
	case dataquery.KindSemanticDistribution:
		return r.executeSemanticDistribution(ctx, request)
	default:
		return dataquery.Result{}, fmt.Errorf("unsupported data query kind %q", request.Kind)
	}
}

func (r *Runtime) executeSemanticAggregate(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	semanticRequest := semanticquery.Request{
		Table:      request.Target,
		Dimensions: dataQueryFields(request.Fields),
		Measures:   dataQueryFields(request.Measures),
		Time:       semanticquery.Time{Field: request.Time.Field, Grain: request.Time.Grain, Alias: request.Time.Alias},
		Filters:    dataQueryFilters(request.Filters),
		Sort:       dataQuerySorts(request.Sort),
		Limit:      request.Limit,
		Offset:     request.Offset,
	}
	plan, err := semanticquery.NewPlanner(r.model).Plan(semanticRequest)
	if err != nil {
		return dataquery.Result{}, err
	}
	rows, err := r.db.Query(ctx, plan)
	if err != nil {
		return dataquery.Result{}, err
	}
	return dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL}, nil
}

func (r *Runtime) executeSemanticRows(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	planner := semanticquery.NewPlanner(r.model)
	if len(request.Fields) == 0 && len(request.Measures) == 0 && request.IncludeTotal {
		countPlan, err := planner.PlanCount(semanticquery.CountRequest{Table: request.Target, Filters: dataQueryFilters(request.Filters)})
		if err != nil {
			return dataquery.Result{}, err
		}
		total, err := r.db.Count(ctx, countPlan)
		if err != nil {
			return dataquery.Result{}, err
		}
		return dataquery.Result{TotalRows: total, TotalRowsKnown: true, SQL: countPlan.SQL}, nil
	}
	semanticRequest := semanticquery.RowRequest{
		Table:      request.Target,
		Dimensions: dataQueryFields(request.Fields),
		Measures:   dataQueryFields(request.Measures),
		Filters:    dataQueryFilters(request.Filters),
		Sort:       dataQuerySorts(request.Sort),
		Limit:      request.Limit,
		Offset:     request.Offset,
	}
	plan, err := planner.PlanRows(semanticRequest)
	if err != nil {
		return dataquery.Result{}, err
	}
	rows, err := r.db.Query(ctx, plan)
	if err != nil {
		return dataquery.Result{}, err
	}
	result := dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL}
	if request.IncludeTotal {
		countPlan, err := planner.PlanCount(semanticquery.CountRequest{Table: request.Target, Filters: dataQueryFilters(request.Filters)})
		if err != nil {
			return dataquery.Result{}, err
		}
		total, err := r.db.Count(ctx, countPlan)
		if err != nil {
			return dataquery.Result{}, err
		}
		result.TotalRows = total
		result.TotalRowsKnown = true
	}
	return result, nil
}

func (r *Runtime) executeModelTableRows(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	plan, err := r.modelTableQueryPlan(ModelTableQuery{
		Table:   request.Target,
		Columns: dataquery.FieldNames(request.Fields),
		Sort:    dataQuerySorts(request.Sort),
		Limit:   request.Limit,
		Offset:  request.Offset,
	})
	if err != nil {
		return dataquery.Result{}, err
	}
	rows, err := r.db.Query(ctx, plan)
	if err != nil {
		return dataquery.Result{}, err
	}
	result := dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL}
	if request.IncludeTotal {
		total, err := r.CountModelTable(ctx, request.Target)
		if err != nil {
			return dataquery.Result{}, err
		}
		result.TotalRows = total
		result.TotalRowsKnown = true
	}
	return result, nil
}

func (r *Runtime) executeSourceRows(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	source, ok := sourceInModel(r.model, request.Target)
	if !ok {
		return dataquery.Result{}, fmt.Errorf("source %q is not available in semantic model %q", request.Target, r.modelID)
	}
	planner, ok := r.sources.(sourceRelationPlanner)
	if !ok {
		return dataquery.Result{}, fmt.Errorf("source %q is not available for raw inspection", request.Target)
	}
	if err := r.sources.PrepareSourceRuntime(ctx, r.model); err != nil {
		return dataquery.Result{}, err
	}
	relation, err := planner.SourceRelation(r.model, source, r.dataDir)
	if err != nil {
		return dataquery.Result{}, err
	}
	columns, err := sourceQueryColumns(source, dataquery.FieldNames(request.Fields))
	if err != nil {
		return dataquery.Result{}, err
	}
	plan, err := rawRelationPlan(relation, columns, request.Sort, request.Offset, request.Limit)
	if err != nil {
		return dataquery.Result{}, err
	}
	rows, err := r.db.Query(ctx, plan)
	if err != nil {
		return dataquery.Result{}, err
	}
	result := dataquery.Result{Columns: dataquery.ColumnsFromNames(plan.Columns), Rows: dataQueryRows(rows), SQL: plan.SQL}
	if request.IncludeTotal {
		total, err := r.db.Count(ctx, semanticquery.Plan{SQL: "WITH data AS (" + relation + ")\nSELECT COUNT(*) FROM data", Columns: []string{"count"}})
		if err != nil {
			return dataquery.Result{}, err
		}
		result.TotalRows = total
		result.TotalRowsKnown = true
	}
	return result, nil
}

func (r *Runtime) executeSemanticHistogram(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	rawRequest := semanticquery.RawValueRequest{
		Table:      request.Target,
		Dimensions: dataQueryFields(request.Fields),
		Measure:    dataQueryFields([]dataquery.Field{request.Value})[0],
		Filters:    dataQueryFilters(request.Filters),
	}
	plan, err := semanticquery.NewPlanner(r.model).PlanRawValues(rawRequest)
	if err != nil {
		return dataquery.Result{}, err
	}
	valueColumn := rawRequest.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	bins, err := r.db.Histogram(ctx, plan, semanticquery.HistogramSpec{
		ValueColumn: valueColumn,
		BinCount:    request.BinCount,
	})
	if err != nil {
		return dataquery.Result{}, err
	}
	rows := make([]dataquery.Row, 0, len(bins))
	for _, bin := range bins {
		rows = append(rows, dataquery.Row{
			"bucket": bin.Bucket,
			"count":  bin.Count,
			"start":  bin.Start,
			"end":    bin.End,
		})
	}
	return dataquery.Result{
		Columns: dataquery.ColumnsFromNames([]string{"bucket", "count", "start", "end"}),
		Rows:    rows,
		SQL:     plan.SQL,
	}, nil
}

func (r *Runtime) executeSemanticDistribution(ctx context.Context, request dataquery.Query) (dataquery.Result, error) {
	rawRequest := semanticquery.RawValueRequest{
		Table:      request.Target,
		Dimensions: dataQueryFields(request.Fields),
		Measure:    dataQueryFields([]dataquery.Field{request.Value})[0],
		Filters:    dataQueryFilters(request.Filters),
	}
	plan, err := semanticquery.NewPlanner(r.model).PlanRawValues(rawRequest)
	if err != nil {
		return dataquery.Result{}, err
	}
	valueColumn := rawRequest.Measure.Alias
	if valueColumn == "" {
		valueColumn = "value"
	}
	groupColumn := "label"
	if len(rawRequest.Dimensions) > 0 && rawRequest.Dimensions[0].Alias != "" {
		groupColumn = rawRequest.Dimensions[0].Alias
	}
	rows, err := r.db.Distribution(ctx, plan, semanticquery.DistributionSpec{
		GroupColumn: groupColumn,
		ValueColumn: valueColumn,
		Sort:        dataQuerySorts(request.Sort),
		Limit:       request.Limit,
	})
	if err != nil {
		return dataquery.Result{}, err
	}
	return dataquery.Result{
		Columns: dataquery.ColumnsFromNames([]string{"label", "min", "q1", "median", "q3", "max"}),
		Rows:    dataQueryRows(rows),
		SQL:     plan.SQL,
	}, nil
}

func (r *Runtime) CountModelTable(ctx context.Context, tableName string) (int, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("materialization runtime is not initialized")
	}
	if _, err := r.modelTable(tableName); err != nil {
		return 0, err
	}
	quotedTable, err := quotedModelTableName(tableName)
	if err != nil {
		return 0, err
	}
	return r.db.Count(ctx, semanticquery.Plan{
		SQL:     "SELECT count(*) FROM model." + quotedTable,
		Columns: []string{"count"},
	})
}

func (r *Runtime) ModelTableRows(ctx context.Context, request ModelTableQuery) (semanticquery.Rows, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("materialization runtime is not initialized")
	}
	plan, err := r.modelTableQueryPlan(request)
	if err != nil {
		return nil, err
	}
	return r.db.Query(ctx, plan)
}

func (r *Runtime) modelTableQueryPlan(request ModelTableQuery) (semanticquery.Plan, error) {
	table, err := r.modelTable(request.Table)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	columns, err := modelTableQueryColumns(table, request.Columns)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	quotedTable, err := quotedModelTableName(request.Table)
	if err != nil {
		return semanticquery.Plan{}, err
	}
	var sql strings.Builder
	sql.WriteString("SELECT ")
	for index, column := range columns {
		if index > 0 {
			sql.WriteString(", ")
		}
		sql.WriteString(quoteMaterializedIdentifier(column))
	}
	sql.WriteString("\nFROM model.")
	sql.WriteString(quotedTable)
	if len(request.Sort) > 0 {
		orderParts := []string{}
		columnSet := modelTableColumnSet(table)
		for _, sortSpec := range request.Sort {
			if !columnSet[sortSpec.Field] {
				return semanticquery.Plan{}, fmt.Errorf("model table %q does not expose sort column %q", request.Table, sortSpec.Field)
			}
			direction := strings.ToUpper(strings.TrimSpace(sortSpec.Direction))
			if direction != "ASC" && direction != "DESC" {
				return semanticquery.Plan{}, fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
			}
			orderParts = append(orderParts, quoteMaterializedIdentifier(sortSpec.Field)+" "+direction)
		}
		if len(orderParts) > 0 {
			sql.WriteString("\nORDER BY ")
			sql.WriteString(strings.Join(orderParts, ", "))
		}
	}
	if request.Limit > 0 {
		sql.WriteString(fmt.Sprintf("\nLIMIT %d", request.Limit))
	}
	if request.Offset > 0 {
		if request.Limit <= 0 {
			return semanticquery.Plan{}, fmt.Errorf("offset requires limit")
		}
		sql.WriteString(fmt.Sprintf("\nOFFSET %d", request.Offset))
	}
	return semanticquery.Plan{SQL: sql.String(), Columns: columns}, nil
}

func (r *Runtime) modelTable(tableName string) (semanticmodel.Table, error) {
	if r == nil || r.model == nil {
		return semanticmodel.Table{}, fmt.Errorf("semantic model is required")
	}
	tableName = strings.TrimSpace(tableName)
	table, ok := r.model.Tables[tableName]
	if !ok {
		return semanticmodel.Table{}, fmt.Errorf("model table %q is not available in semantic model %q", tableName, r.model.Name)
	}
	return table, nil
}

func modelTableQueryColumns(table semanticmodel.Table, requested []string) ([]string, error) {
	columnSet := modelTableColumnSet(table)
	if len(requested) > 0 {
		columns := []string{}
		for _, column := range requested {
			column = strings.TrimSpace(column)
			if column == "" {
				continue
			}
			if !columnSet[column] {
				return nil, fmt.Errorf("model table does not expose column %q", column)
			}
			columns = append(columns, column)
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	if len(table.Schema.Columns) > 0 {
		schemaColumns := append([]semanticmodel.ColumnSchema{}, table.Schema.Columns...)
		sort.SliceStable(schemaColumns, func(i, j int) bool {
			if schemaColumns[i].Ordinal != schemaColumns[j].Ordinal {
				return schemaColumns[i].Ordinal < schemaColumns[j].Ordinal
			}
			return schemaColumns[i].Name < schemaColumns[j].Name
		})
		columns := make([]string, 0, len(schemaColumns))
		for _, column := range schemaColumns {
			if column.Name != "" {
				columns = append(columns, column.Name)
			}
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	columns := make([]string, 0, len(table.Columns))
	for name := range table.Columns {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		return nil, fmt.Errorf("model table has no columns")
	}
	return columns, nil
}

func modelTableColumnSet(table semanticmodel.Table) map[string]bool {
	columns := map[string]bool{}
	for name := range table.Columns {
		columns[name] = true
	}
	for _, column := range table.Schema.Columns {
		if column.Name != "" {
			columns[column.Name] = true
		}
	}
	return columns
}

func quotedModelTableName(tableName string) (string, error) {
	if err := validateIdentifier(tableName); err != nil {
		return "", err
	}
	return quoteMaterializedIdentifier(tableName), nil
}

func rawRelationPlan(relation string, columns []string, sort []dataquery.Sort, offset, limit int) (semanticquery.Plan, error) {
	columnSet := map[string]bool{}
	for _, column := range columns {
		if err := validateIdentifier(column); err != nil {
			return semanticquery.Plan{}, err
		}
		columnSet[column] = true
	}
	var sql strings.Builder
	sql.WriteString("WITH data AS (")
	sql.WriteString(relation)
	sql.WriteString(")\nSELECT ")
	for index, column := range columns {
		if index > 0 {
			sql.WriteString(", ")
		}
		sql.WriteString(quoteMaterializedIdentifier(column))
	}
	sql.WriteString(" FROM data")
	if len(sort) > 0 {
		parts := []string{}
		for _, sortSpec := range sort {
			if !columnSet[sortSpec.Field] {
				return semanticquery.Plan{}, fmt.Errorf("raw data does not expose sort column %q", sortSpec.Field)
			}
			direction := strings.ToUpper(strings.TrimSpace(sortSpec.Direction))
			if direction != "ASC" && direction != "DESC" {
				return semanticquery.Plan{}, fmt.Errorf("unsupported sort direction %q", sortSpec.Direction)
			}
			parts = append(parts, quoteMaterializedIdentifier(sortSpec.Field)+" "+direction)
		}
		if len(parts) > 0 {
			sql.WriteString("\nORDER BY ")
			sql.WriteString(strings.Join(parts, ", "))
		}
	}
	if limit > 0 {
		sql.WriteString(fmt.Sprintf("\nLIMIT %d", limit))
	}
	if offset > 0 {
		if limit <= 0 {
			return semanticquery.Plan{}, fmt.Errorf("offset requires limit")
		}
		sql.WriteString(fmt.Sprintf("\nOFFSET %d", offset))
	}
	return semanticquery.Plan{SQL: sql.String(), Columns: columns}, nil
}

func sourceQueryColumns(source semanticmodel.Source, requested []string) ([]string, error) {
	columnSet := sourceColumnSet(source)
	if len(requested) > 0 {
		columns := []string{}
		for _, column := range requested {
			column = strings.TrimSpace(column)
			if column == "" {
				continue
			}
			if !columnSet[column] {
				return nil, fmt.Errorf("source does not expose column %q", column)
			}
			columns = append(columns, column)
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	if len(source.Schema.Columns) > 0 {
		schemaColumns := append([]semanticmodel.ColumnSchema{}, source.Schema.Columns...)
		sort.SliceStable(schemaColumns, func(i, j int) bool {
			if schemaColumns[i].Ordinal != schemaColumns[j].Ordinal {
				return schemaColumns[i].Ordinal < schemaColumns[j].Ordinal
			}
			return schemaColumns[i].Name < schemaColumns[j].Name
		})
		columns := make([]string, 0, len(schemaColumns))
		for _, column := range schemaColumns {
			if column.Name != "" {
				columns = append(columns, column.Name)
			}
		}
		if len(columns) > 0 {
			return columns, nil
		}
	}
	columns := make([]string, 0, len(source.Fields))
	for name := range source.Fields {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		return nil, fmt.Errorf("source has no columns")
	}
	return columns, nil
}

func sourceColumnSet(source semanticmodel.Source) map[string]bool {
	columns := map[string]bool{}
	for name := range source.Fields {
		columns[name] = true
	}
	for _, column := range source.Schema.Columns {
		if column.Name != "" {
			columns[column.Name] = true
		}
	}
	return columns
}

func sourceInModel(model *semanticmodel.Model, key string) (semanticmodel.Source, bool) {
	if model == nil {
		return semanticmodel.Source{}, false
	}
	key = strings.TrimSpace(key)
	if source, ok := model.Sources[key]; ok {
		return source, true
	}
	localKey := strings.ReplaceAll(key, ".", "_")
	if source, ok := model.Sources[localKey]; ok {
		return source, true
	}
	return semanticmodel.Source{}, false
}

func dataQueryFields(fields []dataquery.Field) []semanticquery.Field {
	out := make([]semanticquery.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, semanticquery.Field{
			Field: field.Field,
			Alias: field.Alias,
			Measure: semanticquery.InlineMeasure{
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

func dataQueryFilters(filters []dataquery.Filter) []semanticquery.Filter {
	out := make([]semanticquery.Filter, 0, len(filters))
	for _, filter := range filters {
		groups := make([]semanticquery.FilterGroup, 0, len(filter.Groups))
		for _, group := range filter.Groups {
			groups = append(groups, semanticquery.FilterGroup{Filters: dataQueryFilters(group.Filters)})
		}
		out = append(out, semanticquery.Filter{
			Field:    filter.Field,
			Operator: filter.Operator,
			Values:   append([]any{}, filter.Values...),
			Groups:   groups,
		})
	}
	return out
}

func dataQuerySorts(sort []dataquery.Sort) []semanticquery.Sort {
	out := make([]semanticquery.Sort, 0, len(sort))
	for _, item := range sort {
		out = append(out, semanticquery.Sort{Field: item.Field, Direction: item.Direction})
	}
	return out
}

func dataQueryRows(rows semanticquery.Rows) []dataquery.Row {
	out := make([]dataquery.Row, 0, len(rows))
	for _, row := range rows {
		converted := dataquery.Row{}
		for key, value := range row {
			converted[key] = value
		}
		out = append(out, converted)
	}
	return out
}

func quoteMaterializedIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (r *Runtime) LastRefresh() time.Time {
	if r == nil {
		return time.Time{}
	}
	return r.lastRefresh
}

func (r *Runtime) DBPath() string {
	if r == nil {
		return ""
	}
	return r.db.Path()
}
