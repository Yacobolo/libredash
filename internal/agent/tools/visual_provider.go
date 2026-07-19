package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	agentcore "github.com/Yacobolo/libredash/pkg/agent"
)

const (
	agentVisualToolName = QueryVisualToolName
	maxVisualRows       = 50
)

type VisualAuthorizeFunc func(ctx context.Context, scope Scope, request VisualAuthorizationRequest) (agentcore.ToolResult, bool)

type VisualModelFunc func(workspaceID, modelID string) (*semanticmodel.Model, bool)

type VisualAggregateRowsFunc func(ctx context.Context, workspaceID, modelID string, request reportdef.AggregateQuery) (reportdef.QueryRows, error)

type VisualPreviewRowsFunc func(ctx context.Context, workspaceID, modelID string, request reportdef.RowQuery) (reportdef.QueryRows, error)

type VisualHistogramFunc func(ctx context.Context, workspaceID, modelID string, request reportdef.RawValueQuery, binCount int) ([]reportdef.HistogramBin, error)

type VisualDistributionFunc func(ctx context.Context, workspaceID, modelID string, request reportdef.RawValueQuery, sort []reportdef.QuerySort, limit int) (reportdef.QueryRows, error)

type VisualProvider struct {
	Authorize     VisualAuthorizeFunc
	SemanticModel VisualModelFunc
	AggregateRows VisualAggregateRowsFunc
	PreviewRows   VisualPreviewRowsFunc
	Histogram     VisualHistogramFunc
	Distribution  VisualDistributionFunc
}

type VisualAuthorizationRequest struct {
	ToolName string
	CallID   string
	Type     string
	Model    string
	Dataset  string
}

type agentVisualInput struct {
	Workspace       string                    `json:"workspace"`
	Model           string                    `json:"model"`
	Dataset         string                    `json:"dataset"`
	Title           string                    `json:"title"`
	Type            string                    `json:"type"`
	Shape           string                    `json:"shape"`
	Dimensions      []agentVisualFieldRef     `json:"dimensions"`
	Series          *agentVisualFieldRef      `json:"series"`
	Measures        []agentVisualFieldRef     `json:"measures"`
	Fields          []agentVisualFieldRef     `json:"fields"`
	Rows            []agentVisualFieldRef     `json:"rows"`
	Columns         []dashboard.TableColumn   `json:"columns"`
	Sort            []agentVisualSort         `json:"sort"`
	Limit           int                       `json:"limit"`
	Options         map[string]any            `json:"options"`
	RendererOptions map[string]map[string]any `json:"rendererOptions"`
}

type agentVisualFieldRef struct {
	Field string `json:"field"`
	Alias string `json:"alias,omitempty"`
}

type agentVisualSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

type agentVisualResult struct {
	Type    string         `json:"type"`
	ID      string         `json:"id"`
	Patch   map[string]any `json:"patch"`
	Summary string         `json:"summary"`
}

func (p VisualProvider) Definitions(scope Scope) []agentcore.ToolDefinition {
	inputSchema := json.RawMessage(agentVisualToolSchema)
	if strings.TrimSpace(scope.WorkspaceID) == "" {
		inputSchema = requireToolStringProperty(inputSchema, "workspace")
	}
	return []agentcore.ToolDefinition{{
		Name:        agentVisualToolName,
		Description: "Create one read-only visual from LibreDash semantic model fields. Data is queried from semantic models; do not provide inline data.",
		InputSchema: inputSchema,
		Handler: agentcore.ToolHandlerFunc(func(ctx context.Context, call agentcore.ToolCall) (agentcore.ToolResult, error) {
			return p.Run(ctx, scope, call), nil
		}),
	}}
}

func (p VisualProvider) Run(ctx context.Context, scope Scope, call agentcore.ToolCall) agentcore.ToolResult {
	if p.Authorize == nil {
		return apigenAgentToolError("authorization_failed", "agent visual tool authorizer is not configured")
	}
	input, err := decodeAgentVisualInput(call.Arguments)
	if err != nil {
		return apigenAgentToolError("invalid_arguments", err.Error())
	}
	runScope := scope
	if runScope.WorkspaceID == "" {
		runScope.WorkspaceID = strings.TrimSpace(input.Workspace)
	}
	if runScope.WorkspaceID == "" {
		return apigenAgentToolError("invalid_arguments", "workspace is required")
	}
	metadata := dataquery.Metadata{
		WorkspaceID: runScope.WorkspaceID,
		Surface:     dataquery.SurfaceAgent,
		Operation:   dataquery.OperationAgentQuery,
		PrincipalID: scope.PrincipalID,
		ObjectType:  "semantic_dataset",
		ObjectID:    input.Model + ":" + input.Dataset,
		RequestID:   call.ID,
	}
	ctx = dataquery.WithMetadata(ctx, metadata)
	if errResult, ok := p.Authorize(ctx, runScope, VisualAuthorizationRequest{
		ToolName: agentVisualToolName,
		CallID:   call.ID,
		Type:     input.Type,
		Model:    input.Model,
		Dataset:  input.Dataset,
	}); !ok {
		return errResult
	}
	result, err := p.queryAgentVisual(ctx, runScope.WorkspaceID, input, agentVisualID(call.ID))
	if err != nil {
		return apigenAgentToolError("query_visual_failed", err.Error())
	}
	return agentcore.ToolResult{
		Content: map[string]any{
			"ok":      true,
			"type":    result.Type,
			"id":      result.ID,
			"summary": result.Summary,
			"signal":  "visuals." + result.ID,
		},
		DisplayContent: result,
	}
}

func decodeAgentVisualInput(rawArgs json.RawMessage) (agentVisualInput, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawArgs, &raw); err != nil {
		return agentVisualInput{}, err
	}
	for _, forbidden := range []string{"filters", "filter", "interaction", "interactions", "data", "values"} {
		if _, ok := raw[forbidden]; ok {
			return agentVisualInput{}, fmt.Errorf("%s is not supported by %s", forbidden, agentVisualToolName)
		}
	}
	var input agentVisualInput
	if err := json.Unmarshal(rawArgs, &input); err != nil {
		return agentVisualInput{}, err
	}
	input.Model = strings.TrimSpace(input.Model)
	input.Dataset = strings.TrimSpace(input.Dataset)
	input.Title = strings.TrimSpace(input.Title)
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Shape = strings.ToLower(strings.TrimSpace(input.Shape))
	if !isAgentVisualType(input.Type) {
		return agentVisualInput{}, fmt.Errorf("type must be a supported visual type")
	}
	if input.Model == "" {
		return agentVisualInput{}, fmt.Errorf("model is required")
	}
	if input.Dataset == "" {
		return agentVisualInput{}, fmt.Errorf("dataset is required")
	}
	input.Limit = agentVisualLimit(input.Limit)
	return input, nil
}

func isAgentVisualType(value string) bool {
	switch value {
	case "line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst", "kpi", "table", "matrix", "pivot":
		return true
	default:
		return false
	}
}

func (p VisualProvider) queryAgentVisual(ctx context.Context, workspaceID string, input agentVisualInput, id string) (agentVisualResult, error) {
	if p.SemanticModel == nil {
		return agentVisualResult{}, fmt.Errorf("semantic model provider is not configured")
	}
	model, ok := p.SemanticModel(workspaceID, input.Model)
	if !ok || model == nil {
		return agentVisualResult{}, fmt.Errorf("unknown semantic model %q", input.Model)
	}
	if _, ok := model.Tables[input.Dataset]; !ok {
		return agentVisualResult{}, fmt.Errorf("unknown dataset %q", input.Dataset)
	}
	switch input.Type {
	case "table", "matrix", "pivot":
		return p.queryAgentTable(ctx, workspaceID, model, input, id)
	default:
		return p.queryAgentChart(ctx, workspaceID, model, input, id)
	}
}

func (p VisualProvider) queryAgentChart(ctx context.Context, workspaceID string, model *semanticmodel.Model, input agentVisualInput, id string) (agentVisualResult, error) {
	shape := agentVisualShape(input)
	input.Shape = shape
	if err := validateAgentChartContract(input); err != nil {
		return agentVisualResult{}, err
	}
	data, err := p.agentChartData(ctx, workspaceID, input, shape, model)
	if err != nil {
		return agentVisualResult{}, err
	}
	measure, _ := model.ResolveMeasure(input.Measures[0].Field)
	title := input.Title
	if title == "" {
		title = measureLabelForAgent(input.Measures[0].Field, measure)
	}
	contract := agentReportVisual(input)
	chartType := input.Type
	visual := dashboard.Visual{
		Version:         3,
		ID:              id,
		Kind:            contract.KindOrDefault(),
		Shape:           shape,
		Renderer:        contract.RendererOrDefault(),
		Type:            chartType,
		Title:           title,
		Unit:            measure.Unit,
		Format:          measure.Format,
		Interaction:     dashboard.InteractionConfig{},
		Dimensions:      displayAgentFields(input.Dimensions),
		Measure:         displayAgentField(input.Measures[0]),
		Measures:        displayAgentFields(input.Measures),
		Series:          agentVisualSeries(input.Series),
		Options:         cloneMap(input.Options),
		RendererOptions: cloneNestedMap(input.RendererOptions),
		Selection:       []dashboard.InteractionSelectionEntry{},
		Data:            data,
	}
	return agentVisualResult{
		Type:    chartType,
		ID:      id,
		Patch:   map[string]any{"visuals": map[string]dashboard.Visual{id: visual}},
		Summary: fmt.Sprintf("Created chart %q with %d data points.", title, len(data)),
	}, nil
}

func agentVisualShape(input agentVisualInput) string {
	return agentReportVisual(input).ShapeOrDefault()
}

func agentReportVisual(input agentVisualInput) reportdef.Visual {
	dimensions := make([]reportdef.FieldRef, len(input.Dimensions))
	for index, field := range input.Dimensions {
		dimensions[index] = reportdef.FieldRef{Field: field.Field, Alias: field.Alias}
	}
	measures := make([]reportdef.FieldRef, len(input.Measures))
	for index, field := range input.Measures {
		measures[index] = reportdef.FieldRef{Field: field.Field, Alias: field.Alias}
	}
	series := reportdef.FieldRef{}
	if input.Series != nil {
		series = reportdef.FieldRef{Field: input.Series.Field, Alias: input.Series.Alias}
	}
	return reportdef.Visual{
		Title: firstNonEmpty(input.Title, "Agent visual"), Shape: input.Shape, Type: input.Type,
		Query:   reportdef.VisualQuery{Table: input.Dataset, Dimensions: dimensions, Series: series, Measures: measures, Limit: input.Limit},
		Options: input.Options,
	}
}

func validateAgentChartContract(input agentVisualInput) error {
	visual := agentReportVisual(input)
	visual.Shape = visual.ShapeOrDefault()
	componentKind := visual.Type + "_chart"
	if visual.Type == "kpi" {
		componentKind = "kpi_card"
	}
	definition := reportdef.Dashboard{
		ID: "agent-visual", Title: "Agent visual", SemanticModel: "agent",
		Visuals: map[string]reportdef.Visual{"visual": visual},
		Pages: []dashboard.Page{{
			ID: "page", Title: "Page",
			Visuals: []dashboard.PageVisual{{ID: "visual", Kind: componentKind, Visual: "visual", Placement: dashboard.PagePlacement{Col: 1, Row: 1, ColSpan: 6, RowSpan: 4}}},
		}},
	}
	if err := definition.ValidateContract(); err != nil {
		return fmt.Errorf("invalid %s visual query: %w", input.Type, err)
	}
	return nil
}

func (p VisualProvider) agentChartData(ctx context.Context, workspaceID string, input agentVisualInput, shape string, model *semanticmodel.Model) ([]dashboard.Datum, error) {
	if shape == "binned_measure" {
		if p.Histogram == nil {
			return nil, fmt.Errorf("histogram query provider is not configured")
		}
		binCount := 20
		if value, ok := input.Options["bin_count"].(float64); ok {
			binCount = max(5, min(60, int(value)))
		} else if value, ok := input.Options["bin_count"].(int); ok {
			binCount = max(5, min(60, value))
		}
		bins, err := p.Histogram(ctx, workspaceID, input.Model, reportdef.RawValueQuery{
			Table: input.Dataset, Measure: reportdef.QueryField{Field: input.Measures[0].Field, Alias: "value"},
		}, binCount)
		if err != nil {
			return nil, err
		}
		out := make([]dashboard.Datum, 0, len(bins))
		for _, bin := range bins {
			out = append(out, dashboard.Datum{"label": fmt.Sprintf("%g–%g", bin.Start, bin.End), "binStart": bin.Start, "binEnd": bin.End, "value": bin.Count})
		}
		return out, nil
	}
	if shape == "distribution" {
		if p.Distribution == nil {
			return nil, fmt.Errorf("distribution query provider is not configured")
		}
		rows, err := p.Distribution(ctx, workspaceID, input.Model, reportdef.RawValueQuery{
			Table:      input.Dataset,
			Dimensions: []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "label"}},
			Measure:    reportdef.QueryField{Field: input.Measures[0].Field, Alias: "value"},
		}, agentVisualSorts(input.Sort, input.Dimensions, input.Series, input.Measures), input.Limit)
		return agentDatums(rows), err
	}
	if p.AggregateRows == nil {
		return nil, fmt.Errorf("aggregate query provider is not configured")
	}
	if shape == "single_value" {
		rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
			Table:    input.Dataset,
			Measures: []reportdef.QueryField{{Field: input.Measures[0].Field, Alias: "value"}},
			Limit:    1,
		})
		if err != nil {
			return nil, err
		}
		value := any(nil)
		if len(rows) > 0 {
			value = agentRowValue(rows[0], "value", input.Measures[0])
		}
		return []dashboard.Datum{{"label": firstNonEmpty(input.Title, measureLabelForAgent(input.Measures[0].Field, mustResolveMeasure(model, input.Measures[0].Field))), "value": value}}, nil
	}
	if shape == "category_multi_measure" || len(input.Measures) > 1 {
		if shape == "ohlc" {
			aliases := []string{"open", "close", "low", "high"}
			measures := make([]reportdef.QueryField, len(input.Measures))
			for index, measure := range input.Measures {
				measures[index] = reportdef.QueryField{Field: measure.Field, Alias: aliases[index]}
			}
			rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
				Table: input.Dataset, Dimensions: []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "label"}},
				Measures: measures, Sort: agentVisualSorts(input.Sort, input.Dimensions, input.Series, input.Measures), Limit: input.Limit,
			})
			return agentDatums(rows), err
		}
		out := []dashboard.Datum{}
		for _, measureRef := range input.Measures {
			rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
				Table:      input.Dataset,
				Dimensions: []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "label"}},
				Measures:   []reportdef.QueryField{{Field: measureRef.Field, Alias: "value"}},
				Sort:       agentVisualSorts(input.Sort, input.Dimensions, input.Series, []agentVisualFieldRef{measureRef}),
				Limit:      input.Limit,
			})
			if err != nil {
				return nil, err
			}
			measure, _ := model.ResolveMeasure(measureRef.Field)
			for _, row := range rows {
				out = append(out, dashboard.Datum{
					"label":  agentRowValue(row, "label", input.Dimensions[0]),
					"series": measureLabelForAgent(measureRef.Field, measure),
					"value":  agentRowValue(row, "value", measureRef),
				})
			}
		}
		return out, nil
	}
	if shape == "hierarchy" {
		dimensions := make([]reportdef.QueryField, len(input.Dimensions))
		for index, dimension := range input.Dimensions {
			dimensions[index] = reportdef.QueryField{Field: dimension.Field, Alias: fmt.Sprintf("level_%d", index)}
		}
		rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
			Table: input.Dataset, Dimensions: dimensions, Measures: []reportdef.QueryField{{Field: input.Measures[0].Field, Alias: "value"}}, Limit: input.Limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]dashboard.Datum, 0, len(rows))
		for _, row := range rows {
			path := make([]string, 0, len(dimensions))
			for index := range dimensions {
				if value := fmt.Sprint(row[fmt.Sprintf("level_%d", index)]); value != "" && value != "<nil>" {
					path = append(path, value)
				}
			}
			out = append(out, dashboard.Datum{"path": path, "value": row["value"]})
		}
		return out, nil
	}
	if shape == "matrix" || shape == "graph" {
		left, right := "row", "column"
		if shape == "graph" {
			left, right = "source", "target"
		}
		rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
			Table:      input.Dataset,
			Dimensions: []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: left}, {Field: input.Dimensions[1].Field, Alias: right}},
			Measures:   []reportdef.QueryField{{Field: input.Measures[0].Field, Alias: "value"}}, Limit: input.Limit,
		})
		return agentDatums(rows), err
	}
	if shape == "geo" {
		rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
			Table: input.Dataset, Dimensions: []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "name"}},
			Measures: []reportdef.QueryField{{Field: input.Measures[0].Field, Alias: "value"}}, Limit: input.Limit,
		})
		return agentDatums(rows), err
	}
	dimensions := []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "label"}}
	if input.Series != nil && input.Series.Field != "" {
		dimensions = append(dimensions, reportdef.QueryField{Field: input.Series.Field, Alias: "series"})
	}
	rows, err := p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
		Table:      input.Dataset,
		Dimensions: dimensions,
		Measures:   []reportdef.QueryField{{Field: input.Measures[0].Field, Alias: "value"}},
		Sort:       agentVisualSorts(input.Sort, input.Dimensions, input.Series, input.Measures),
		Limit:      input.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dashboard.Datum, 0, len(rows))
	for _, row := range rows {
		datum := dashboard.Datum{"label": agentRowValue(row, "label", input.Dimensions[0]), "value": agentRowValue(row, "value", input.Measures[0])}
		if input.Series != nil && input.Series.Field != "" {
			datum["series"] = agentRowValue(row, "series", *input.Series)
		}
		out = append(out, datum)
	}
	if shape == "category_delta" {
		cumulative := 0.0
		for _, datum := range out {
			value := agentFloat(datum["value"])
			datum["start"] = cumulative
			cumulative += value
			datum["end"] = cumulative
			datum["positive"] = value >= 0
		}
	}
	return out, nil
}

func agentDatums(rows reportdef.QueryRows) []dashboard.Datum {
	out := make([]dashboard.Datum, 0, len(rows))
	for _, row := range rows {
		datum := dashboard.Datum{}
		for key, value := range row {
			datum[key] = value
		}
		out = append(out, datum)
	}
	return out
}

func agentFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func (p VisualProvider) queryAgentTable(ctx context.Context, workspaceID string, model *semanticmodel.Model, input agentVisualInput, id string) (agentVisualResult, error) {
	fields := input.Fields
	aggregate := len(fields) == 0 && (len(input.Rows) > 0 || len(input.Measures) > 0)
	if len(fields) == 0 {
		fields = append([]agentVisualFieldRef{}, input.Rows...)
		fields = append(fields, input.Measures...)
	}
	if len(fields) == 0 {
		return agentVisualResult{}, fmt.Errorf("table requires fields, or rows and measures")
	}
	dimensions, measures, columns, err := agentTableFields(model, fields, input.Columns)
	if err != nil {
		return agentVisualResult{}, err
	}
	var rows reportdef.QueryRows
	if aggregate {
		if p.AggregateRows == nil {
			return agentVisualResult{}, fmt.Errorf("aggregate query provider is not configured")
		}
		rows, err = p.AggregateRows(ctx, workspaceID, input.Model, reportdef.AggregateQuery{
			Table:      input.Dataset,
			Dimensions: dimensions,
			Measures:   measures,
			Sort:       agentTableSorts(input.Sort, fields),
			Limit:      input.Limit,
		})
	} else {
		if p.PreviewRows == nil {
			return agentVisualResult{}, fmt.Errorf("preview query provider is not configured")
		}
		rows, err = p.PreviewRows(ctx, workspaceID, input.Model, reportdef.RowQuery{
			Table:      input.Dataset,
			Dimensions: dimensions,
			Measures:   measures,
			Sort:       agentTableSorts(input.Sort, fields),
			Limit:      input.Limit,
		})
	}
	if err != nil {
		return agentVisualResult{}, err
	}
	tableRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, map[string]any(row))
	}
	title := firstNonEmpty(input.Title, "Table")
	sortSpec := dashboard.TableSort{}
	if len(input.Sort) > 0 {
		sortSpec = dashboard.TableSort{Key: agentFieldAlias(input.Sort[0].Field), Direction: normalizedSortDirection(input.Sort[0].Direction)}
	}
	internalType := map[string]string{"table": "data_table", "matrix": "matrix_table", "pivot": "pivot_table"}[input.Type]
	table := dashboard.Table{
		Version:       2,
		Kind:          internalType,
		Title:         title,
		Style:         dashboard.TableStyle{}.WithDefaults(),
		Interaction:   dashboard.InteractionConfig{},
		Selection:     []dashboard.InteractionSelectionEntry{},
		Columns:       columns,
		Cardinality:   dashboard.ExactCardinality(len(tableRows)),
		AvailableRows: len(tableRows),
		IsCapped:      false,
		RowCap:        maxVisualRows,
		ChunkSize:     dashboard.TableChunkSize,
		RowHeight:     dashboard.TableRowHeight,
		ResetVersion:  0,
		Sort:          sortSpec,
		Blocks: map[string]dashboard.TableBlock{
			"a": {Start: 0, RequestSeq: 0, ResetVersion: 0, Sort: sortSpec, Rows: tableRows},
		},
		LoadingBlock: "",
		Error:        "",
	}
	return agentVisualResult{
		Type:    input.Type,
		ID:      id,
		Patch:   map[string]any{"visuals": map[string]dashboard.TabularVisual{id: dashboard.NewTabularVisual(id, table)}},
		Summary: fmt.Sprintf("Created table %q with %d rows.", title, len(tableRows)),
	}, nil
}

func agentTableFields(model *semanticmodel.Model, fields []agentVisualFieldRef, overrides []dashboard.TableColumn) ([]reportdef.QueryField, []reportdef.QueryField, []dashboard.TableColumn, error) {
	dimensions := []reportdef.QueryField{}
	measures := []reportdef.QueryField{}
	columns := make([]dashboard.TableColumn, 0, len(fields))
	overrideByKey := map[string]dashboard.TableColumn{}
	for _, column := range overrides {
		if column.Key != "" {
			overrideByKey[column.Key] = column
		}
	}
	for _, field := range fields {
		if strings.TrimSpace(field.Field) == "" {
			return nil, nil, nil, fmt.Errorf("table field is required")
		}
		alias := agentFieldAliasForRef(field)
		if dimension, err := model.ResolveDimension(field.Field); err == nil {
			dimensions = append(dimensions, reportdef.QueryField{Field: field.Field, Alias: alias})
			columns = append(columns, mergeAgentTableColumn(dashboard.TableColumn{Key: alias, Label: dimensionLabelForAgent(alias, dimension), Format: "text"}, overrideByKey[alias]))
			continue
		}
		measure, err := model.ResolveMeasure(field.Field)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unknown field %q", field.Field)
		}
		measures = append(measures, reportdef.QueryField{Field: field.Field, Alias: alias})
		columns = append(columns, mergeAgentTableColumn(dashboard.TableColumn{Key: alias, Label: measureLabelForAgent(field.Field, measure), Align: "right", Role: "measure", Measure: alias, Format: measure.Format}, overrideByKey[alias]))
	}
	return dimensions, measures, columns, nil
}

func agentVisualLimit(limit int) int {
	if limit <= 0 || limit > maxVisualRows {
		return maxVisualRows
	}
	return limit
}

func agentVisualSorts(sorts []agentVisualSort, dimensions []agentVisualFieldRef, series *agentVisualFieldRef, measures []agentVisualFieldRef) []reportdef.QuerySort {
	if len(sorts) == 0 {
		return []reportdef.QuerySort{{Field: "label", Direction: "asc"}}
	}
	out := make([]reportdef.QuerySort, 0, len(sorts))
	for _, sortSpec := range sorts {
		field := sortSpec.Field
		if agentSortMatches(field, dimensions) {
			field = "label"
		} else if series != nil && agentSortMatches(field, []agentVisualFieldRef{*series}) {
			field = "series"
		} else if agentSortMatches(field, measures) {
			field = "value"
		}
		out = append(out, reportdef.QuerySort{Field: field, Direction: normalizedSortDirection(sortSpec.Direction)})
	}
	return out
}

func agentTableSorts(sorts []agentVisualSort, fields []agentVisualFieldRef) []reportdef.QuerySort {
	out := make([]reportdef.QuerySort, 0, len(sorts))
	for _, sortSpec := range sorts {
		field := agentFieldAlias(sortSpec.Field)
		for _, ref := range fields {
			if sortSpec.Field == ref.Field || sortSpec.Field == ref.Alias {
				field = agentFieldAliasForRef(ref)
				break
			}
		}
		out = append(out, reportdef.QuerySort{Field: field, Direction: normalizedSortDirection(sortSpec.Direction)})
	}
	return out
}

func agentSortMatches(field string, refs []agentVisualFieldRef) bool {
	for _, ref := range refs {
		if field == ref.Field || field == ref.Alias || field == agentFieldAlias(ref.Field) {
			return true
		}
	}
	return false
}

func agentRowValue(row map[string]any, alias string, ref agentVisualFieldRef) any {
	for _, key := range []string{alias, ref.Alias, agentFieldAlias(ref.Field), ref.Field} {
		if key == "" {
			continue
		}
		if value, ok := row[key]; ok {
			return value
		}
	}
	return nil
}

func normalizedSortDirection(direction string) string {
	if strings.ToLower(direction) == "desc" {
		return "desc"
	}
	return "asc"
}

func agentVisualID(seed string) string {
	suffix := sanitizeAgentVisualIDSeed(seed)
	if suffix == "" {
		suffix = randomAgentVisualIDSuffix()
	}
	return "agent_visual_" + suffix
}

func sanitizeAgentVisualIDSeed(seed string) string {
	seed = strings.TrimSpace(strings.ToLower(seed))
	var b strings.Builder
	for _, r := range seed {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 48 {
			break
		}
	}
	return strings.Trim(b.String(), "_-")
}

func randomAgentVisualIDSuffix() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(bytes[:])
}

func displayAgentFields(fields []agentVisualFieldRef) []string {
	out := make([]string, len(fields))
	for i, field := range fields {
		out[i] = displayAgentField(field)
	}
	return out
}

func displayAgentField(field agentVisualFieldRef) string {
	if field.Alias != "" {
		return field.Alias
	}
	return agentFieldAlias(field.Field)
}

func agentVisualSeries(series *agentVisualFieldRef) []string {
	if series == nil || series.Field == "" {
		return []string{}
	}
	return []string{displayAgentField(*series)}
}

func agentFieldAliasForRef(field agentVisualFieldRef) string {
	if field.Alias != "" {
		return field.Alias
	}
	return agentFieldAlias(field.Field)
}

func agentFieldAlias(field string) string {
	parts := strings.Split(field, ".")
	return parts[len(parts)-1]
}

func dimensionLabelForAgent(fallback string, dimension semanticmodel.MetricDimension) string {
	if dimension.Label != "" {
		return dimension.Label
	}
	return fallback
}

func measureLabelForAgent(fallback string, measure semanticmodel.MetricMeasure) string {
	if measure.Label != "" {
		return measure.Label
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mustResolveMeasure(model *semanticmodel.Model, field string) semanticmodel.MetricMeasure {
	measure, _ := model.ResolveMeasure(field)
	return measure
}

func mergeAgentTableColumn(base, override dashboard.TableColumn) dashboard.TableColumn {
	if override.Key != "" {
		base.Key = override.Key
	}
	if override.Label != "" {
		base.Label = override.Label
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	if override.Role != "" {
		base.Role = override.Role
	}
	if override.Group != "" {
		base.Group = override.Group
	}
	if override.Measure != "" {
		base.Measure = override.Measure
	}
	if override.ColumnValue != "" {
		base.ColumnValue = override.ColumnValue
	}
	if override.Width > 0 {
		base.Width = override.Width
	}
	if override.Format != "" {
		base.Format = override.Format
	}
	if len(override.Formatting) > 0 {
		base.Formatting = append([]dashboard.TableFormattingRule{}, override.Formatting...)
	}
	return base
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneNestedMap(in map[string]map[string]any) map[string]map[string]any {
	if len(in) == 0 {
		return map[string]map[string]any{}
	}
	out := make(map[string]map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneMap(value)
	}
	return out
}

const agentVisualToolSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["type", "model", "dataset"],
  "properties": {
    "model": {"type": "string", "minLength": 1, "description": "Semantic model ID."},
    "dataset": {"type": "string", "minLength": 1, "description": "Semantic dataset/table ID."},
    "title": {"type": "string", "description": "Optional display title."},
    "type": {"type": "string", "enum": ["line", "area", "bar", "column", "pie", "donut", "scatter", "funnel", "treemap", "gauge", "heatmap", "sankey", "graph", "map", "candlestick", "boxplot", "combo", "waterfall", "histogram", "radar", "tree", "sunburst", "kpi", "table", "matrix", "pivot"], "description": "Visual type."},
    "shape": {"type": "string", "description": "Optional visual shape override."},
    "dimensions": {
      "type": "array",
      "description": "Dimension fields for chart grouping.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["field"],
        "properties": {
          "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
          "alias": {"type": "string", "description": "Optional output alias."}
        }
      }
    },
    "series": {
      "type": "object",
      "additionalProperties": false,
      "required": ["field"],
      "description": "Optional series field for split charts.",
      "properties": {
        "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
        "alias": {"type": "string", "description": "Optional output alias."}
      }
    },
    "measures": {
      "type": "array",
      "description": "Measure fields for chart values.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["field"],
        "properties": {
          "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
          "alias": {"type": "string", "description": "Optional output alias."}
        }
      }
    },
    "fields": {
      "type": "array",
      "description": "Fields for table output.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["field"],
        "properties": {
          "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
          "alias": {"type": "string", "description": "Optional output alias."}
        }
      }
    },
    "rows": {
      "type": "array",
      "description": "Row fields for matrix-style table output.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["field"],
        "properties": {
          "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
          "alias": {"type": "string", "description": "Optional output alias."}
        }
      }
    },
    "columns": {"type": "array", "description": "Optional table column display configuration.", "items": {"type": "object", "additionalProperties": true}},
    "sort": {
      "type": "array",
      "description": "Sort fields applied to the query result.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["field"],
        "properties": {
          "field": {"type": "string", "minLength": 1, "description": "Semantic field ID."},
          "direction": {"type": "string", "enum": ["asc", "desc"], "description": "Sort direction."}
        }
      }
    },
    "limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum result rows."},
    "options": {"type": "object", "additionalProperties": true, "description": "Renderer-neutral options."},
    "rendererOptions": {
      "type": "object",
      "description": "Renderer-specific options keyed by renderer name.",
      "additionalProperties": {"type": "object", "additionalProperties": true}
    }
  }
}`
