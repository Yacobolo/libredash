package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/access"
	"github.com/Yacobolo/libredash/internal/agentapp"
	"github.com/Yacobolo/libredash/internal/agenttools"
	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard"
	reportdef "github.com/Yacobolo/libredash/internal/dashboard/report"
	"github.com/Yacobolo/libredash/internal/dataquery"
	"github.com/Yacobolo/libredash/pkg/agent"
)

const agentVisualToolName = agenttools.QueryVisualToolName

type agentVisualInput struct {
	Kind            string                    `json:"kind"`
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
	Kind    string         `json:"kind"`
	ID      string         `json:"id"`
	Patch   map[string]any `json:"patch"`
	Summary string         `json:"summary"`
}

func (s *Server) agentVisualToolDefinitions(scope agentapp.Scope) []agent.ToolDefinition {
	return []agent.ToolDefinition{{
		Name:        agentVisualToolName,
		Description: "Create one read-only chart or BI table artifact from LibreDash semantic model fields. Data is queried from semantic models; do not provide inline data.",
		InputSchema: json.RawMessage(agentVisualToolSchema),
		Handler: agent.ToolHandlerFunc(func(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error) {
			return s.runAgentVisualTool(ctx, scope, call), nil
		}),
	}}
}

func (s *Server) runAgentVisualTool(ctx context.Context, scope agentapp.Scope, call agent.ToolCall) agent.ToolResult {
	if errResult, ok := s.authorizeAgentPermission(ctx, scope, access.PermissionAssetRead); !ok {
		return errResult
	}
	input, err := decodeAgentVisualInput(call.Arguments)
	if err != nil {
		return apigenAgentToolError("invalid_arguments", err.Error())
	}
	metadata := dataquery.Metadata{
		WorkspaceID: scope.WorkspaceID,
		Surface:     dataquery.SurfaceAgent,
		Operation:   dataquery.OperationAgentQuery,
		PrincipalID: scope.PrincipalID,
		ObjectType:  "semantic_dataset",
		ObjectID:    input.Model + ":" + input.Dataset,
		RequestID:   call.ID,
	}
	ctx = dataquery.WithMetadata(ctx, metadata)
	result, err := s.queryAgentVisual(ctx, input, agentVisualID(input.Kind, call.ID))
	if err != nil {
		return apigenAgentToolError("query_visual_failed", err.Error())
	}
	return agent.ToolResult{
		Content: map[string]any{
			"ok":      true,
			"kind":    result.Kind,
			"id":      result.ID,
			"summary": result.Summary,
			"signal":  agentVisualSignal(result.Kind, result.ID),
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
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	input.Model = strings.TrimSpace(input.Model)
	input.Dataset = strings.TrimSpace(input.Dataset)
	input.Title = strings.TrimSpace(input.Title)
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	input.Shape = strings.ToLower(strings.TrimSpace(input.Shape))
	if input.Kind != "chart" && input.Kind != "table" {
		return agentVisualInput{}, fmt.Errorf("kind must be chart or table")
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

func (s *Server) queryAgentVisual(ctx context.Context, input agentVisualInput, id string) (agentVisualResult, error) {
	model, ok := s.metrics.SemanticModel(input.Model)
	if !ok || model == nil {
		return agentVisualResult{}, fmt.Errorf("unknown semantic model %q", input.Model)
	}
	if _, ok := model.Tables[input.Dataset]; !ok {
		return agentVisualResult{}, fmt.Errorf("unknown dataset %q", input.Dataset)
	}
	switch input.Kind {
	case "chart":
		return s.queryAgentChart(ctx, model, input, id)
	case "table":
		return s.queryAgentTable(ctx, model, input, id)
	default:
		return agentVisualResult{}, fmt.Errorf("kind must be chart or table")
	}
}

func (s *Server) queryAgentChart(ctx context.Context, model *semanticmodel.Model, input agentVisualInput, id string) (agentVisualResult, error) {
	if len(input.Measures) == 0 {
		return agentVisualResult{}, fmt.Errorf("chart requires at least one measure")
	}
	shape := input.Shape
	if shape == "" {
		if len(input.Dimensions) == 0 {
			shape = "single_value"
		} else if input.Series != nil && input.Series.Field != "" {
			shape = "category_series_value"
		} else if len(input.Measures) > 1 {
			shape = "category_multi_measure"
		} else {
			shape = "category_value"
		}
	}
	if shape != "single_value" && len(input.Dimensions) == 0 {
		return agentVisualResult{}, fmt.Errorf("chart shape %q requires at least one dimension", shape)
	}
	data, err := s.agentChartData(ctx, input, shape, model)
	if err != nil {
		return agentVisualResult{}, err
	}
	measure, _ := model.ResolveMeasure(input.Measures[0].Field)
	title := input.Title
	if title == "" {
		title = measureLabelForAgent(input.Measures[0].Field, measure)
	}
	chartType := input.Type
	if chartType == "" {
		if shape == "single_value" {
			chartType = "gauge"
		} else {
			chartType = "bar"
		}
	}
	if chartType == "kpi" {
		return agentVisualResult{}, fmt.Errorf("kpi cards are not supported by %s", agentVisualToolName)
	}
	visual := dashboard.Visual{
		Version:         3,
		ID:              id,
		Kind:            "chart",
		Shape:           shape,
		Renderer:        "echarts",
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
		Kind:    "chart",
		ID:      id,
		Patch:   map[string]any{"visuals": map[string]dashboard.Visual{id: visual}},
		Summary: fmt.Sprintf("Created chart %q with %d data points.", title, len(data)),
	}, nil
}

func (s *Server) agentChartData(ctx context.Context, input agentVisualInput, shape string, model *semanticmodel.Model) ([]dashboard.Datum, error) {
	if shape == "single_value" {
		rows, err := executeAggregateRows(ctx, s.metrics, input.Model, reportdef.AggregateQuery{
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
		out := []dashboard.Datum{}
		for _, measureRef := range input.Measures {
			rows, err := executeAggregateRows(ctx, s.metrics, input.Model, reportdef.AggregateQuery{
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
	dimensions := []reportdef.QueryField{{Field: input.Dimensions[0].Field, Alias: "label"}}
	if input.Series != nil && input.Series.Field != "" {
		dimensions = append(dimensions, reportdef.QueryField{Field: input.Series.Field, Alias: "series"})
	}
	rows, err := executeAggregateRows(ctx, s.metrics, input.Model, reportdef.AggregateQuery{
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
	return out, nil
}

func (s *Server) queryAgentTable(ctx context.Context, model *semanticmodel.Model, input agentVisualInput, id string) (agentVisualResult, error) {
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
		rows, err = executeAggregateRows(ctx, s.metrics, input.Model, reportdef.AggregateQuery{
			Table:      input.Dataset,
			Dimensions: dimensions,
			Measures:   measures,
			Sort:       agentTableSorts(input.Sort, fields),
			Limit:      input.Limit,
		})
	} else {
		rows, err = executePreviewRows(ctx, s.metrics, input.Model, reportdef.RowQuery{
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
	table := dashboard.Table{
		Version:       2,
		Kind:          "data_table",
		Title:         title,
		Style:         dashboard.TableStyle{}.WithDefaults(),
		Interaction:   dashboard.InteractionConfig{},
		Selection:     []dashboard.InteractionSelectionEntry{},
		Columns:       columns,
		TotalRows:     len(tableRows),
		AvailableRows: len(tableRows),
		IsCapped:      false,
		RowCap:        maxAgentRows,
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
		Kind:    "table",
		ID:      id,
		Patch:   map[string]any{"tables": map[string]dashboard.Table{id: table}},
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
	if limit <= 0 || limit > maxAgentRows {
		return maxAgentRows
	}
	return limit
}

func agentVisualSignal(kind, id string) string {
	switch kind {
	case "chart":
		return "visuals." + id
	case "table":
		return "tables." + id
	default:
		return id
	}
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

func agentVisualID(kind, seed string) string {
	suffix := sanitizeAgentVisualIDSeed(seed)
	if suffix == "" {
		suffix = randomAgentVisualIDSuffix()
	}
	return "agent_" + kind + "_" + suffix
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
  "required": ["kind", "model", "dataset"],
  "properties": {
    "kind": {"type": "string", "enum": ["chart", "table"], "description": "Artifact kind to create."},
    "model": {"type": "string", "minLength": 1, "description": "Semantic model ID."},
    "dataset": {"type": "string", "minLength": 1, "description": "Semantic dataset/table ID."},
    "title": {"type": "string", "description": "Optional display title."},
    "type": {"type": "string", "description": "Optional chart or table renderer type."},
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
