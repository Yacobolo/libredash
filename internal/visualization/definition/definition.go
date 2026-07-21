// Package definition owns immutable, compiler-produced visualization
// definitions. It deliberately contains no authoring YAML or renderer-native
// configuration.
package definition

import (
	"fmt"

	"github.com/Yacobolo/libredash/internal/visualization/ir"
)

const (
	RendererECharts  = "echarts"
	RendererTanStack = "tanstack"
	RendererHTML     = "html"
	RendererMapLibre = "maplibre"
	RendererVegaLite = "vega-lite-sandbox"
)

type QueryKind string

const (
	QueryAggregate QueryKind = "aggregate"
	QueryDetail    QueryKind = "detail"
	QueryMatrix    QueryKind = "matrix"
	QueryPivot     QueryKind = "pivot"
	QueryCustom    QueryKind = "custom"
	QuerySpatial   QueryKind = "spatial"
)

// QueryBinding is the closed compiler/runtime boundary. Exactly one branch is
// present and must match Kind. It contains stable semantic member IDs and
// compiler-resolved output aliases; authoring query objects never cross this
// boundary.
type QueryBinding struct {
	Kind      QueryKind `json:"kind" yaml:"kind"`
	ModelID   string    `json:"modelID" yaml:"model_id"`
	DatasetID string    `json:"datasetID" yaml:"dataset_id"`
	Identity  []string  `json:"identity,omitempty" yaml:"identity,omitempty"`

	Aggregate *AggregateQueryBinding `json:"aggregate,omitempty" yaml:"aggregate,omitempty"`
	Detail    *DetailQueryBinding    `json:"detail,omitempty" yaml:"detail,omitempty"`
	Matrix    *MatrixQueryBinding    `json:"matrix,omitempty" yaml:"matrix,omitempty"`
	Pivot     *PivotQueryBinding     `json:"pivot,omitempty" yaml:"pivot,omitempty"`
	Custom    *CustomQueryBinding    `json:"custom,omitempty" yaml:"custom,omitempty"`
	Spatial   *SpatialQueryBinding   `json:"spatial,omitempty" yaml:"spatial,omitempty"`
}

type FieldBinding struct {
	FieldID string `json:"fieldID" yaml:"field_id"`
	Alias   string `json:"alias" yaml:"alias"`
}

type TimeBinding struct {
	FieldID string `json:"fieldID" yaml:"field_id"`
	Alias   string `json:"alias" yaml:"alias"`
	Grain   string `json:"grain" yaml:"grain"`
}

type AggregateQueryBinding struct {
	TableID    string         `json:"tableID" yaml:"table_id"`
	Dimensions []FieldBinding `json:"dimensions" yaml:"dimensions"`
	Series     *FieldBinding  `json:"series,omitempty" yaml:"series,omitempty"`
	Measures   []FieldBinding `json:"measures" yaml:"measures"`
	Time       *TimeBinding   `json:"time,omitempty" yaml:"time,omitempty"`
	Sort       []Sort         `json:"sort,omitempty" yaml:"sort,omitempty"`
	Limit      int64          `json:"limit" yaml:"limit"`
}

type DetailQueryBinding struct {
	TableID     string         `json:"tableID" yaml:"table_id"`
	Fields      []FieldBinding `json:"fields" yaml:"fields"`
	DefaultSort []Sort         `json:"defaultSort,omitempty" yaml:"default_sort,omitempty"`
	Limit       int64          `json:"limit" yaml:"limit"`
}

type MatrixQueryBinding struct {
	TableID  string         `json:"tableID" yaml:"table_id"`
	Rows     []FieldBinding `json:"rows" yaml:"rows"`
	Columns  []FieldBinding `json:"columns" yaml:"columns"`
	Measures []FieldBinding `json:"measures" yaml:"measures"`
	Limit    int64          `json:"limit" yaml:"limit"`
}

type PivotQueryBinding = MatrixQueryBinding

type CustomQueryBinding struct {
	TableID string         `json:"tableID" yaml:"table_id"`
	Fields  []FieldBinding `json:"fields" yaml:"fields"`
	Sort    []Sort         `json:"sort,omitempty" yaml:"sort,omitempty"`
	Limit   int64          `json:"limit" yaml:"limit"`
}

// SpatialQueryBinding is the compiler-resolved query contract for a
// geographic visualization. Viewport is present only when the visual uses the
// large-data spatial runtime; inline and keyed choropleth maps deliberately
// keep it nil while retaining the same geographic query ownership.
type SpatialQueryBinding struct {
	TableID    string                  `json:"tableID" yaml:"table_id"`
	Dimensions []FieldBinding          `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
	Series     *FieldBinding           `json:"series,omitempty" yaml:"series,omitempty"`
	Measures   []FieldBinding          `json:"measures,omitempty" yaml:"measures,omitempty"`
	Time       *TimeBinding            `json:"time,omitempty" yaml:"time,omitempty"`
	Sort       []Sort                  `json:"sort,omitempty" yaml:"sort,omitempty"`
	Limit      int64                   `json:"limit" yaml:"limit"`
	Viewport   *SpatialViewportBinding `json:"viewport,omitempty" yaml:"viewport,omitempty"`
}

// SpatialViewportBinding identifies the one compiler-resolved coordinate pair
// used to govern viewport requests. All coordinate layers in a windowed map
// must use this pair.
type SpatialViewportBinding struct {
	Latitude       FieldBinding `json:"latitude" yaml:"latitude"`
	Longitude      FieldBinding `json:"longitude" yaml:"longitude"`
	FeatureCap     int64        `json:"featureCap" yaml:"feature_cap"`
	RawMinimumZoom float64      `json:"rawMinimumZoom" yaml:"raw_minimum_zoom"`
}

type Sort struct {
	FieldID   string `json:"fieldID" yaml:"field_id"`
	Direction string `json:"direction" yaml:"direction"`
}

func (query QueryBinding) Validate() error {
	if query.Kind == "" || query.ModelID == "" || query.DatasetID == "" {
		return fmt.Errorf("visualization query binding requires kind, model ID, and dataset ID")
	}
	branches := 0
	for _, present := range []bool{query.Aggregate != nil, query.Detail != nil, query.Matrix != nil, query.Pivot != nil, query.Custom != nil, query.Spatial != nil} {
		if present {
			branches++
		}
	}
	if branches != 1 {
		return fmt.Errorf("visualization query binding requires exactly one query branch, got %d", branches)
	}
	var tableID string
	var fields []FieldBinding
	var limit int64
	switch query.Kind {
	case QueryAggregate:
		if query.Aggregate == nil {
			return fmt.Errorf("aggregate query binding requires aggregate branch")
		}
		tableID, fields, limit = query.Aggregate.TableID, query.Aggregate.Measures, query.Aggregate.Limit
	case QueryDetail:
		if query.Detail == nil {
			return fmt.Errorf("detail query binding requires detail branch")
		}
		tableID, fields, limit = query.Detail.TableID, query.Detail.Fields, query.Detail.Limit
	case QueryMatrix:
		if query.Matrix == nil {
			return fmt.Errorf("matrix query binding requires matrix branch")
		}
		tableID, fields, limit = query.Matrix.TableID, query.Matrix.Measures, query.Matrix.Limit
	case QueryPivot:
		if query.Pivot == nil {
			return fmt.Errorf("pivot query binding requires pivot branch")
		}
		tableID, fields, limit = query.Pivot.TableID, query.Pivot.Measures, query.Pivot.Limit
	case QueryCustom:
		if query.Custom == nil {
			return fmt.Errorf("custom query binding requires custom branch")
		}
		tableID, fields, limit = query.Custom.TableID, query.Custom.Fields, query.Custom.Limit
	case QuerySpatial:
		if query.Spatial == nil {
			return fmt.Errorf("spatial query binding requires spatial branch")
		}
		tableID = query.Spatial.TableID
		fields = append(fields, query.Spatial.Dimensions...)
		if query.Spatial.Series != nil {
			fields = append(fields, *query.Spatial.Series)
		}
		if query.Spatial.Time != nil {
			fields = append(fields, FieldBinding{FieldID: query.Spatial.Time.FieldID, Alias: query.Spatial.Time.Alias})
		}
		fields = append(fields, query.Spatial.Measures...)
		limit = query.Spatial.Limit
		if viewport := query.Spatial.Viewport; viewport != nil {
			if viewport.FeatureCap <= 0 || viewport.FeatureCap > limit {
				return fmt.Errorf("spatial viewport requires a positive feature cap no greater than its row limit")
			}
			if viewport.RawMinimumZoom < 0 || viewport.RawMinimumZoom > 24 {
				return fmt.Errorf("spatial viewport raw minimum zoom must be between 0 and 24")
			}
			if !containsFieldBinding(fields, viewport.Latitude) || !containsFieldBinding(fields, viewport.Longitude) {
				return fmt.Errorf("spatial viewport coordinates must reference compiled query fields")
			}
		}
	default:
		return fmt.Errorf("unsupported visualization query kind %q", query.Kind)
	}
	if (query.Kind == QueryDetail && tableID == "") || len(fields) == 0 || limit <= 0 {
		return fmt.Errorf("visualization %s query requires fields and positive limit", query.Kind)
	}
	for index, field := range fields {
		if field.FieldID == "" || field.Alias == "" {
			return fmt.Errorf("visualization %s query field %d requires field ID and alias", query.Kind, index)
		}
	}
	return nil
}

func containsFieldBinding(fields []FieldBinding, target FieldBinding) bool {
	if target.FieldID == "" || target.Alias == "" {
		return false
	}
	for _, field := range fields {
		if field == target {
			return true
		}
	}
	return false
}

type Definition struct {
	ID           string               `json:"id" yaml:"id"`
	RendererID   string               `json:"rendererID" yaml:"renderer_id"`
	SpecRevision string               `json:"specRevision" yaml:"spec_revision"`
	Spec         ir.VisualizationSpec `json:"spec" yaml:"spec"`
	Query        QueryBinding         `json:"query" yaml:"query"`
}

func New(id string, spec ir.VisualizationSpec, query QueryBinding) (Definition, error) {
	renderer, expectedQuery, err := ownership(spec)
	if err != nil {
		return Definition{}, err
	}
	if query.Kind == "" {
		query.Kind = expectedQuery
	}
	revision, err := ir.ComputeSpecRevision(spec)
	if err != nil {
		return Definition{}, fmt.Errorf("compute visualization %q specification revision: %w", id, err)
	}
	definition := Definition{ID: id, RendererID: renderer, SpecRevision: revision.String(), Spec: spec, Query: query}
	if err := definition.Validate(); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

func (definition Definition) Validate() error {
	if definition.ID == "" || definition.RendererID == "" || definition.Query.Kind == "" {
		return fmt.Errorf("visualization definition requires ID, renderer, and query kind")
	}
	if err := definition.Query.Validate(); err != nil {
		return fmt.Errorf("visualization %q query: %w", definition.ID, err)
	}
	if err := ir.ValidateSpec(definition.Spec); err != nil {
		return fmt.Errorf("visualization %q specification: %w", definition.ID, err)
	}
	renderer, queryKind, err := ownership(definition.Spec)
	if err != nil {
		return err
	}
	if definition.RendererID != renderer {
		return fmt.Errorf("visualization %q renderer %q, want %q", definition.ID, definition.RendererID, renderer)
	}
	if definition.Query.Kind != queryKind {
		return fmt.Errorf("visualization %q query kind %q, want %q", definition.ID, definition.Query.Kind, queryKind)
	}
	revision, err := ir.ComputeSpecRevision(definition.Spec)
	if err != nil {
		return err
	}
	if definition.SpecRevision != revision.String() {
		return fmt.Errorf("visualization %q specification revision mismatch", definition.ID)
	}
	return nil
}

func ownership(spec ir.VisualizationSpec) (string, QueryKind, error) {
	switch spec.Value.(type) {
	case *ir.CartesianVisualizationSpec, ir.CartesianVisualizationSpec,
		*ir.ProportionalVisualizationSpec, ir.ProportionalVisualizationSpec,
		*ir.HierarchyVisualizationSpec, ir.HierarchyVisualizationSpec,
		*ir.PolarVisualizationSpec, ir.PolarVisualizationSpec:
		return RendererECharts, QueryAggregate, nil
	case *ir.TableVisualizationSpec, ir.TableVisualizationSpec:
		return RendererTanStack, QueryDetail, nil
	case *ir.MatrixVisualizationSpec, ir.MatrixVisualizationSpec:
		return RendererTanStack, QueryMatrix, nil
	case *ir.PivotVisualizationSpec, ir.PivotVisualizationSpec:
		return RendererTanStack, QueryPivot, nil
	case *ir.KPIVisualizationSpec, ir.KPIVisualizationSpec:
		return RendererHTML, QueryAggregate, nil
	case *ir.GeographicVisualizationSpec, ir.GeographicVisualizationSpec:
		return RendererMapLibre, QuerySpatial, nil
	case *ir.CustomVisualizationSpec, ir.CustomVisualizationSpec:
		return RendererVegaLite, QueryCustom, nil
	default:
		return "", "", fmt.Errorf("unsupported visualization specification %T", spec.Value)
	}
}
