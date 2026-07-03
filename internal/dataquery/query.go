package dataquery

import (
	"context"
	"fmt"
	"strings"
)

type Kind string

const (
	KindSemanticAggregate    Kind = "semantic_aggregate"
	KindSemanticRows         Kind = "semantic_rows"
	KindModelTableRows       Kind = "model_table_rows"
	KindSourceRows           Kind = "source_rows"
	KindSemanticHistogram    Kind = "semantic_histogram"
	KindSemanticDistribution Kind = "semantic_distribution"
)

type Query struct {
	WorkspaceID   string
	Surface       string
	Operation     string
	PrincipalID   string
	RequestID     string
	ObjectType    string
	ObjectID      string
	CorrelationID string
	ModelID       string
	Kind          Kind
	Target        string
	Fields        []Field
	Measures      []Field
	Value         Field
	Time          Time
	Filters       []Filter
	Sort          []Sort
	Offset        int
	Limit         int
	BinCount      int
	IncludeTotal  bool
}

type Field struct {
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

type Time struct {
	Field string
	Grain string
	Alias string
}

type Filter struct {
	Field    string
	Operator string
	Values   []any
	Groups   []FilterGroup
}

type FilterGroup struct {
	Filters []Filter
}

type Sort struct {
	Field     string
	Direction string
}

type Column struct {
	Name string
}

type Row map[string]any

type Result struct {
	Columns        []Column
	Rows           []Row
	TotalRows      int
	TotalRowsKnown bool
	SQL            string
	PlanText       string
	DurationMS     int64
	QueueWaitMS    int64
	ExecutionMS    int64
	ExecutionState string
	RowsReturned   int
	BytesEstimate  int64
	Status         string
	Error          string
	Warnings       []string
}

const (
	SurfaceDashboard    = "dashboard"
	SurfaceAPI          = "api"
	SurfaceAgent        = "agent"
	SurfaceCLI          = "cli"
	SurfaceDataExplorer = "data_explorer"

	OperationDashboardAggregate    = "dashboard_aggregate"
	OperationDashboardRows         = "dashboard_rows"
	OperationDashboardCount        = "dashboard_count"
	OperationDashboardHistogram    = "dashboard_histogram"
	OperationDashboardDistribution = "dashboard_distribution"
	OperationAPIQuery              = "api_query"
	OperationAPIPreview            = "api_preview"
	OperationAgentQuery            = "agent_query"
	OperationPreviewWindow         = "preview_window"

	StatusSuccess  = "success"
	StatusError    = "error"
	StatusCanceled = "canceled"
	StatusTimeout  = "timeout"

	ExecutionStarted   = "started"
	ExecutionRejected  = "rejected"
	ExecutionCanceled  = "canceled"
	ExecutionTimeout   = "timeout"
	ExecutionSucceeded = "succeeded"
	ExecutionFailed    = "failed"
)

type Metadata struct {
	WorkspaceID   string
	Surface       string
	Operation     string
	PrincipalID   string
	RequestID     string
	ObjectType    string
	ObjectID      string
	CorrelationID string
}

type metadataContextKey struct{}

func WithMetadata(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, metadataContextKey{}, metadata)
}

func MetadataFromContext(ctx context.Context) Metadata {
	metadata, _ := ctx.Value(metadataContextKey{}).(Metadata)
	return metadata
}

func (q Query) WithMetadata(metadata Metadata) Query {
	if metadata.WorkspaceID != "" {
		q.WorkspaceID = metadata.WorkspaceID
	}
	if metadata.Surface != "" {
		q.Surface = metadata.Surface
	}
	if metadata.Operation != "" {
		q.Operation = metadata.Operation
	}
	if metadata.PrincipalID != "" {
		q.PrincipalID = metadata.PrincipalID
	}
	if metadata.RequestID != "" {
		q.RequestID = metadata.RequestID
	}
	if metadata.ObjectType != "" {
		q.ObjectType = metadata.ObjectType
	}
	if metadata.ObjectID != "" {
		q.ObjectID = metadata.ObjectID
	}
	if metadata.CorrelationID != "" {
		q.CorrelationID = metadata.CorrelationID
	}
	return q
}

func SemanticAggregate(modelID, target string, fields, measures []Field, filters []Filter, sort []Sort, offset, limit int) Query {
	return Query{ModelID: modelID, Kind: KindSemanticAggregate, Target: target, Fields: fields, Measures: measures, Filters: filters, Sort: sort, Offset: offset, Limit: limit}
}

func SemanticRows(modelID, target string, fields, measures []Field, filters []Filter, sort []Sort, offset, limit int, includeTotal bool) Query {
	return Query{ModelID: modelID, Kind: KindSemanticRows, Target: target, Fields: fields, Measures: measures, Filters: filters, Sort: sort, Offset: offset, Limit: limit, IncludeTotal: includeTotal}
}

func ModelTableRows(modelID, table string, columns []string, sort []Sort, offset, limit int, includeTotal bool) Query {
	return Query{ModelID: modelID, Kind: KindModelTableRows, Target: table, Fields: fieldsFromNames(columns), Sort: sort, Offset: offset, Limit: limit, IncludeTotal: includeTotal}
}

func SourceRows(modelID, source string, columns []string, sort []Sort, offset, limit int, includeTotal bool) Query {
	return Query{ModelID: modelID, Kind: KindSourceRows, Target: source, Fields: fieldsFromNames(columns), Sort: sort, Offset: offset, Limit: limit, IncludeTotal: includeTotal}
}

func SemanticHistogram(modelID, target string, dimensions []Field, measure Field, filters []Filter, binCount int) Query {
	return Query{ModelID: modelID, Kind: KindSemanticHistogram, Target: target, Fields: dimensions, Value: measure, Filters: filters, BinCount: binCount}
}

func SemanticDistribution(modelID, target string, dimensions []Field, measure Field, filters []Filter, sort []Sort, limit int) Query {
	return Query{ModelID: modelID, Kind: KindSemanticDistribution, Target: target, Fields: dimensions, Value: measure, Filters: filters, Sort: sort, Limit: limit}
}

func (q Query) Validate() error {
	if strings.TrimSpace(q.ModelID) == "" {
		return fmt.Errorf("data query requires model id")
	}
	if q.Offset < 0 {
		return fmt.Errorf("data query offset must be non-negative")
	}
	if q.Limit < 0 {
		return fmt.Errorf("data query limit must be non-negative")
	}
	for _, sort := range q.Sort {
		if strings.TrimSpace(sort.Field) == "" {
			return fmt.Errorf("data query sort field is required")
		}
		direction := strings.ToLower(strings.TrimSpace(sort.Direction))
		if direction != "" && direction != "asc" && direction != "desc" {
			return fmt.Errorf("unsupported sort direction %q", sort.Direction)
		}
	}
	switch q.Kind {
	case KindSemanticAggregate, KindSemanticRows:
		if len(q.Fields) == 0 && len(q.Measures) == 0 && q.Time.Field == "" && !(q.Kind == KindSemanticRows && q.IncludeTotal) {
			return fmt.Errorf("%s query requires at least one selected field", q.Kind)
		}
	case KindModelTableRows, KindSourceRows:
		if strings.TrimSpace(q.Target) == "" {
			return fmt.Errorf("%s query requires target", q.Kind)
		}
	case KindSemanticHistogram, KindSemanticDistribution:
		if strings.TrimSpace(q.Target) == "" {
			return fmt.Errorf("%s query requires target", q.Kind)
		}
		if strings.TrimSpace(q.Value.Field) == "" && strings.TrimSpace(q.Value.Measure.Name) == "" {
			return fmt.Errorf("%s query requires a value field", q.Kind)
		}
		if q.Kind == KindSemanticHistogram && q.BinCount <= 0 {
			return fmt.Errorf("semantic histogram query requires a positive bin count")
		}
	default:
		return fmt.Errorf("unsupported data query kind %q", q.Kind)
	}
	return nil
}

func FieldNames(fields []Field) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field.Field)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func ColumnsFromNames(names []string) []Column {
	out := make([]Column, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, Column{Name: name})
		}
	}
	return out
}

func fieldsFromNames(names []string) []Field {
	out := make([]Field, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, Field{Field: name})
		}
	}
	return out
}
