// Package runtime shapes governed query results into the visualization IR.
package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	visualizationdefinition "github.com/Yacobolo/libredash/internal/visualization/definition"
	visualizationgeometry "github.com/Yacobolo/libredash/internal/visualization/geometry"
	"github.com/Yacobolo/libredash/internal/visualization/ir"
)

const primaryDataset = "primary"

func VisualEnvelope(visual dashboard.Visual, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
	shape := runtimeShape(visual)
	data := normalizedVisualData(shape, visual.Data)
	columns := visualColumns(shape, data)
	schema := schemaFromData(columns, data)
	markIdentityFields(&schema, visual.Interaction)
	base := ir.VisualizationSpecBase{
		Title: defaultText(visual.Title, visual.ID), Datasets: []ir.VisualizationDatasetSchema{schema},
		DataBudget:    ir.VisualizationDataBudget{MaxRows: max(int64(len(data)), 1), RequiredCompleteness: ir.VisualizationCompletenessComplete},
		Accessibility: ir.VisualizationAccessibility{Title: defaultText(visual.Title, visual.ID), Description: defaultText(visual.Title, visual.ID)},
		Interactions:  interactions(visual, schema),
	}
	spec, renderer, err := visualSpec(visual, base, columns)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	revision, err := ir.ComputeSpecRevision(spec)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	rows := make([][]any, len(data))
	for index, datum := range data {
		rows[index] = row(columns, datum)
	}
	state := ir.InlineVisualizationDataState{VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "inline", SpecRevision: revision.String(), DataRevision: dataRevision, Generation: generation}, Kind: "inline", Datasets: []ir.VisualizationInlineDataset{{ID: primaryDataset, SpecRevision: revision.String(), DataRevision: dataRevision, Generation: generation, Columns: columns, Rows: rows, Completeness: completeness(rows)}}}
	envelope := ir.VisualizationEnvelope{SchemaVersion: ir.CurrentSchemaVersion, VisualID: visual.ID, RendererID: renderer, SpecRevision: revision.String(), Spec: spec, DataRevision: dataRevision, DataState: ir.VisualizationDataState{Value: &state}, Selection: []ir.VisualizationSelectionEntry{}, Status: ir.VisualizationStatus{Kind: statusKind(len(rows), "")}, Diagnostics: []ir.VisualizationDiagnostic{}}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("visualization %q: %w", visual.ID, err)
	}
	return envelope, nil
}

// VisualEnvelopeFromDefinition shapes runtime data while retaining the exact
// immutable specification and renderer selected by the compiler.
func VisualEnvelopeFromDefinition(definition visualizationdefinition.Definition, visual dashboard.Visual, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
	if err := definition.Validate(); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if visual.ID != definition.ID {
		return ir.VisualizationEnvelope{}, fmt.Errorf("runtime visualization %q does not match compiled definition %q", visual.ID, definition.ID)
	}
	base, err := ir.SpecificationBase(definition.Spec)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	schema, err := compiledDatasetSchema(base, definition.Query.DatasetID)
	if err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled visualization %q: %w", definition.ID, err)
	}
	columns := make([]string, len(schema.Fields))
	for index, field := range schema.Fields {
		columns[index] = field.ID
	}
	data := normalizeCompiledData(schema, visual.Data)
	rows := make([][]any, len(data))
	for index, datum := range data {
		rows[index] = row(columns, datum)
	}
	state := ir.InlineVisualizationDataState{
		VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "inline", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
		Kind:                       "inline",
		Datasets: []ir.VisualizationInlineDataset{{
			ID: definition.Query.DatasetID, SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation,
			Columns: columns, Rows: rows, Completeness: completeness(rows),
		}},
	}
	envelope := ir.VisualizationEnvelope{
		SchemaVersion: ir.CurrentSchemaVersion, VisualID: definition.ID, RendererID: definition.RendererID,
		SpecRevision: definition.SpecRevision, Spec: definition.Spec, DataRevision: dataRevision,
		DataState: ir.VisualizationDataState{Value: &state}, Selection: []ir.VisualizationSelectionEntry{},
		Status: ir.VisualizationStatus{Kind: statusKind(len(rows), "")}, Diagnostics: []ir.VisualizationDiagnostic{},
	}
	envelope.Selection, err = compiledSelections(definition.Spec, visual.Selection, dataRevision)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled visualization %q: %w", definition.ID, err)
	}
	return envelope, nil
}

func compiledDatasetSchema(base ir.VisualizationSpecBase, datasetID string) (ir.VisualizationDatasetSchema, error) {
	for _, schema := range base.Datasets {
		if schema.ID == datasetID {
			return schema, nil
		}
	}
	return ir.VisualizationDatasetSchema{}, fmt.Errorf("query targets unknown dataset %q", datasetID)
}

func normalizeCompiledData(schema ir.VisualizationDatasetSchema, values []dashboard.Datum) []dashboard.Datum {
	hasNode, hasParent := containsField(schema, "node"), containsField(schema, "parent")
	out := make([]dashboard.Datum, len(values))
	for index, value := range values {
		next := dashboard.Datum{}
		for key, item := range value {
			if key != "selected" {
				next[key] = item
			}
		}
		if hasNode && hasParent {
			if path, ok := next["path"].([]string); ok && len(path) > 0 {
				next["node"] = strings.Join(path, "/")
				if len(path) > 1 {
					next["parent"] = strings.Join(path[:len(path)-1], "/")
				}
				delete(next, "path")
			}
		}
		out[index] = next
	}
	return out
}

// TableEnvelopeFromDefinition shapes a window while retaining the exact
// immutable grid specification selected by the compiler.
func TableEnvelopeFromDefinition(definition visualizationdefinition.Definition, table dashboard.Table, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
	if err := definition.Validate(); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	base, err := ir.SpecificationBase(definition.Spec)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if len(table.Columns) == 0 {
		table.Columns = []dashboard.TableColumn{{Key: "value", Label: "Value"}}
	}
	schema := compiledWindowSchema(base, definition.Query.DatasetID, table)
	if table.Sort.Key == "" {
		table.Sort.Key = schema.Fields[0].ID
	}
	sortValue := ir.VisualizationSort{Field: ir.VisualizationFieldRef{Dataset: definition.Query.DatasetID, Field: table.Sort.Key}, Direction: sortDirection(table.Sort.Direction)}
	blocks := make(map[string]ir.VisualizationWindowBlock, len(table.Blocks))
	fieldNames := make([]string, len(schema.Fields))
	for index, field := range schema.Fields {
		fieldNames[index] = field.ID
	}
	for key, block := range table.Blocks {
		if len(block.Rows) == 0 || block.Start >= table.AvailableRows {
			continue
		}
		if excess := block.Start + len(block.Rows) - table.AvailableRows; excess > 0 {
			block.Rows = block.Rows[:len(block.Rows)-excess]
		}
		if block.Sort.Key == "" {
			block.Sort = table.Sort
		}
		rows := make([][]any, len(block.Rows))
		for index, value := range block.Rows {
			rows[index] = row(fieldNames, value)
		}
		blocks[key] = ir.VisualizationWindowBlock{
			ID: key, Start: int64(block.Start), Rows: rows, RequestSeq: int64(block.RequestSeq), ResetVersion: int64(block.ResetVersion),
			Sort: []ir.VisualizationSort{{Field: ir.VisualizationFieldRef{Dataset: definition.Query.DatasetID, Field: block.Sort.Key}, Direction: sortDirection(block.Sort.Direction)}},
		}
	}
	cardinality := ir.VisualizationCardinality{Kind: cardinalityKind(table.Cardinality.Kind)}
	if cardinality.Kind != ir.VisualizationCardinalityKindUnknown {
		count := int64(table.Cardinality.Value)
		cardinality.Count = &count
	}
	state := ir.WindowedVisualizationDataState{
		VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "windowed", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
		Kind:                       "windowed", Schema: schema, Cardinality: cardinality, AvailableRows: int64(table.AvailableRows), RowCap: base.DataBudget.MaxRows,
		ChunkSize: int64(max(table.ChunkSize, dashboard.TableChunkSize)), ResetVersion: int64(table.ResetVersion), Sort: []ir.VisualizationSort{sortValue}, Blocks: blocks,
	}
	message := table.Error
	envelope := ir.VisualizationEnvelope{
		SchemaVersion: ir.CurrentSchemaVersion, VisualID: definition.ID, RendererID: definition.RendererID,
		SpecRevision: definition.SpecRevision, Spec: definition.Spec, DataRevision: dataRevision,
		DataState: ir.VisualizationDataState{Value: &state}, Selection: []ir.VisualizationSelectionEntry{},
		Status: ir.VisualizationStatus{Kind: statusKind(table.AvailableRows, message), Message: optional(message)}, Diagnostics: []ir.VisualizationDiagnostic{},
	}
	envelope.Selection, err = compiledSelections(definition.Spec, table.Selection, dataRevision)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled visualization %q: %w", definition.ID, err)
	}
	return envelope, nil
}

func compiledWindowSchema(base ir.VisualizationSpecBase, datasetID string, table dashboard.Table) ir.VisualizationDatasetSchema {
	compiledFields := map[string]ir.VisualizationField{}
	for _, dataset := range base.Datasets {
		if dataset.ID != datasetID {
			continue
		}
		for _, field := range dataset.Fields {
			compiledFields[field.ID] = field
		}
	}
	fields := make([]ir.VisualizationField, len(table.Columns))
	for index, column := range table.Columns {
		if field, ok := compiledFields[column.Key]; ok {
			fields[index] = field
			continue
		}
		role := ir.VisualizationFieldRoleDimension
		if column.Role == "row_header" && index == 0 {
			role = ir.VisualizationFieldRoleIdentity
		} else if column.Role == "measure" || column.Align == "right" {
			role = ir.VisualizationFieldRoleMeasure
		}
		fields[index] = ir.VisualizationField{
			ID: column.Key, Role: role, DataType: tableDataType(column, table), Nullable: true,
			Label: defaultText(column.Label, column.Key), Format: tableFormat(column),
			Grid: &ir.VisualizationGridFieldMetadata{Group: optional(column.Group), Measure: optional(column.Measure), ColumnValue: optional(column.ColumnValue), Formatting: tableFormatting(column.Formatting)},
		}
	}
	return ir.VisualizationDatasetSchema{ID: datasetID, Fields: fields}
}

// EmptyEnvelopeFromDefinition creates the initial renderer boundary without
// reconstructing any legacy chart or table presentation model.
func EmptyEnvelopeFromDefinition(definition visualizationdefinition.Definition, dataRevision, generation, resetVersion int64) (ir.VisualizationEnvelope, error) {
	if err := definition.Validate(); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	base, err := ir.SpecificationBase(definition.Spec)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	schema, err := compiledDatasetSchema(base, definition.Query.DatasetID)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	envelope := ir.VisualizationEnvelope{
		SchemaVersion: ir.CurrentSchemaVersion, VisualID: definition.ID, RendererID: definition.RendererID,
		SpecRevision: definition.SpecRevision, Spec: definition.Spec, DataRevision: dataRevision,
		Selection: []ir.VisualizationSelectionEntry{}, Status: ir.VisualizationStatus{Kind: ir.VisualizationStatusKindNoData}, Diagnostics: []ir.VisualizationDiagnostic{},
	}
	if definition.Query.Kind == visualizationdefinition.QueryDetail || definition.Query.Kind == visualizationdefinition.QueryMatrix || definition.Query.Kind == visualizationdefinition.QueryPivot {
		sort := emptyWindowSort(definition.Spec, schema)
		state := ir.WindowedVisualizationDataState{
			VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "windowed", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
			Kind:                       "windowed", Schema: schema, Cardinality: ir.VisualizationCardinality{Kind: ir.VisualizationCardinalityKindUnknown},
			AvailableRows: 0, RowCap: base.DataBudget.MaxRows, ChunkSize: dashboard.TableChunkSize, ResetVersion: resetVersion,
			Sort: sort, Blocks: map[string]ir.VisualizationWindowBlock{},
		}
		envelope.DataState = ir.VisualizationDataState{Value: &state}
	} else {
		columns := make([]string, len(schema.Fields))
		for index, field := range schema.Fields {
			columns[index] = field.ID
		}
		state := ir.InlineVisualizationDataState{
			VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "inline", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
			Kind:                       "inline", Datasets: []ir.VisualizationInlineDataset{{
				ID: schema.ID, SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation,
				Columns: columns, Rows: [][]any{}, Completeness: ir.VisualizationCompletenessEmpty,
			}},
		}
		envelope.DataState = ir.VisualizationDataState{Value: &state}
	}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled visualization %q: %w", definition.ID, err)
	}
	return envelope, nil
}

func emptyWindowSort(spec ir.VisualizationSpec, schema ir.VisualizationDatasetSchema) []ir.VisualizationSort {
	if value, ok := spec.Value.(*ir.TableVisualizationSpec); ok && value.DefaultSort != nil && len(*value.DefaultSort) > 0 {
		return append([]ir.VisualizationSort(nil), (*value.DefaultSort)...)
	}
	if len(schema.Fields) == 0 {
		return []ir.VisualizationSort{}
	}
	return []ir.VisualizationSort{{Field: ir.VisualizationFieldRef{Dataset: schema.ID, Field: schema.Fields[0].ID}, Direction: ir.VisualizationSortDirectionAscending}}
}

func compiledSelections(spec ir.VisualizationSpec, entries []dashboard.InteractionSelectionEntry, dataRevision int64) ([]ir.VisualizationSelectionEntry, error) {
	interactions := specInteractions(spec)
	if len(entries) == 0 || len(interactions) == 0 {
		return []ir.VisualizationSelectionEntry{}, nil
	}
	interaction := interactions[0]
	out := make([]ir.VisualizationSelectionEntry, 0, len(entries))
	for index, entry := range entries {
		identity := map[string]any{}
		datasetID := ""
		for _, mapping := range interaction.Mappings {
			if datasetID == "" {
				datasetID = mapping.Source.Dataset
			} else if datasetID != mapping.Source.Dataset {
				return nil, fmt.Errorf("selection %d spans multiple datasets", index)
			}
			matched := false
			for _, selected := range entry.Mappings {
				fact, grain := "", ""
				if mapping.TargetFactID != nil {
					fact = *mapping.TargetFactID
				}
				if mapping.Grain != nil {
					grain = *mapping.Grain
				}
				if selected.Field == mapping.TargetFieldID && selected.Fact == fact && selected.Grain == grain {
					identity[mapping.Source.Field] = selected.Value
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("selection %d omits compiled mapping for %q", index, mapping.TargetFieldID)
			}
		}
		label := optional(entry.Label)
		out = append(out, ir.VisualizationSelectionEntry{Datum: ir.VisualizationDatumRef{Dataset: datasetID, DataRevision: dataRevision, Identity: identity}, Label: label})
	}
	return out, nil
}

func specInteractions(spec ir.VisualizationSpec) []ir.VisualizationInteraction {
	switch value := spec.Value.(type) {
	case *ir.CartesianVisualizationSpec:
		return value.Interactions
	case *ir.ProportionalVisualizationSpec:
		return value.Interactions
	case *ir.HierarchyVisualizationSpec:
		return value.Interactions
	case *ir.PolarVisualizationSpec:
		return value.Interactions
	case *ir.TableVisualizationSpec:
		return value.Interactions
	case *ir.MatrixVisualizationSpec:
		return value.Interactions
	case *ir.PivotVisualizationSpec:
		return value.Interactions
	case *ir.KPIVisualizationSpec:
		return value.Interactions
	case *ir.GeographicVisualizationSpec:
		return value.Interactions
	case *ir.CustomVisualizationSpec:
		return value.Interactions
	default:
		return nil
	}
}

func runtimeShape(visual dashboard.Visual) string {
	if visual.Shape != "" {
		return visual.Shape
	}
	switch visual.Type {
	case "kpi", "gauge":
		return "single_value"
	case "combo":
		return "category_multi_measure"
	case "waterfall":
		return "category_delta"
	case "histogram":
		return "binned_measure"
	case "tree", "sunburst", "treemap":
		return "hierarchy"
	case "heatmap":
		return "matrix"
	case "sankey", "graph":
		return "graph"
	case "map":
		return "geo"
	case "candlestick":
		return "ohlc"
	case "boxplot":
		return "distribution"
	default:
		return "category_value"
	}
}

func TableEnvelope(id string, table dashboard.Table, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
	if len(table.Columns) == 0 {
		table.Columns = []dashboard.TableColumn{{Key: "value", Label: "Value"}}
	}
	fields := make([]ir.VisualizationField, len(table.Columns))
	columns := make([]ir.TableVisualizationColumn, len(table.Columns))
	for index, column := range table.Columns {
		role := ir.VisualizationFieldRoleDimension
		if column.Role == "row_header" && index == 0 {
			role = ir.VisualizationFieldRoleIdentity
		} else if column.Role == "measure" || column.Align == "right" {
			role = ir.VisualizationFieldRoleMeasure
		}
		fields[index] = ir.VisualizationField{
			ID: column.Key, Role: role, DataType: tableDataType(column, table), Nullable: true,
			Label: defaultText(column.Label, column.Key), Format: tableFormat(column),
			Grid: &ir.VisualizationGridFieldMetadata{Group: optional(column.Group), Measure: optional(column.Measure), ColumnValue: optional(column.ColumnValue), Formatting: tableFormatting(column.Formatting)},
		}
		width := int64(column.Width)
		columns[index] = ir.TableVisualizationColumn{
			Field: ref(column.Key), Label: defaultText(column.Label, column.Key),
			Group: optional(column.Group), Measure: optional(column.Measure), ColumnValue: optional(column.ColumnValue),
			Formatting: tableFormatting(column.Formatting),
		}
		if width > 0 {
			columns[index].Width = &width
		}
	}
	schema := ir.VisualizationDatasetSchema{ID: primaryDataset, Fields: fields}
	markIdentityFields(&schema, table.Interaction)
	maximumRows := int64(table.RowCap)
	if maximumRows <= 0 {
		maximumRows = dashboard.TableInteractiveRowCap
	}
	rowHeight := table.RowHeight
	if rowHeight <= 0 {
		rowHeight = dashboard.TableRowHeight
	}
	presentation := ir.GridVisualizationPresentation{RowHeight: int64(rowHeight), Striped: table.Style.Zebra == nil || *table.Style.Zebra, ShowHeader: true}
	base := ir.VisualizationSpecBase{Kind: "table", Title: defaultText(table.Title, id), Datasets: []ir.VisualizationDatasetSchema{schema}, DataBudget: ir.VisualizationDataBudget{MaxRows: maximumRows, RequiredCompleteness: ir.VisualizationCompletenessPartial}, Accessibility: ir.VisualizationAccessibility{Title: defaultText(table.Title, id), Description: defaultText(table.Title, id)}, Interactions: runtimeInteractions(table.Interaction, schema)}
	if table.Sort.Key == "" {
		table.Sort.Key = table.Columns[0].Key
	}
	sortValue := ir.VisualizationSort{Field: ref(table.Sort.Key), Direction: sortDirection(table.Sort.Direction)}
	typeName := map[string]string{"matrix_table": "matrix", "pivot_table": "pivot"}[table.Kind]
	if typeName == "" {
		typeName = "table"
	}
	var spec ir.VisualizationSpec
	switch typeName {
	case "matrix":
		rows, cols, measures := gridRefs(fields)
		base.Kind = "matrix"
		spec = ir.VisualizationSpec{Value: &ir.MatrixVisualizationSpec{VisualizationSpecBase: base, Kind: "matrix", Rows: rows, Columns: cols, Measures: measures, MeasureFormatting: map[string][]ir.TableVisualizationFormattingRule{}, Presentation: presentation}}
	case "pivot":
		rows, cols, measures := gridRefs(fields)
		base.Kind = "pivot"
		spec = ir.VisualizationSpec{Value: &ir.PivotVisualizationSpec{VisualizationSpecBase: base, Kind: "pivot", Rows: rows, Columns: cols, Measures: measures, MeasureFormatting: map[string][]ir.TableVisualizationFormattingRule{}, Presentation: presentation}}
	default:
		spec = ir.VisualizationSpec{Value: &ir.TableVisualizationSpec{VisualizationSpecBase: base, Kind: "table", Columns: columns, DefaultSort: &[]ir.VisualizationSort{sortValue}, Presentation: presentation}}
	}
	revision, err := ir.ComputeSpecRevision(spec)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	blocks := make(map[string]ir.VisualizationWindowBlock, len(table.Blocks))
	fieldNames := make([]string, len(fields))
	for index, field := range fields {
		fieldNames[index] = field.ID
	}
	for key, block := range table.Blocks {
		if len(block.Rows) == 0 || block.Start >= table.AvailableRows {
			continue
		}
		if excess := block.Start + len(block.Rows) - table.AvailableRows; excess > 0 {
			block.Rows = block.Rows[:len(block.Rows)-excess]
		}
		if block.Sort.Key == "" {
			block.Sort = table.Sort
		}
		rows := make([][]any, len(block.Rows))
		for index, value := range block.Rows {
			rows[index] = row(fieldNames, value)
		}
		blocks[key] = ir.VisualizationWindowBlock{ID: key, Start: int64(block.Start), Rows: rows, RequestSeq: int64(block.RequestSeq), ResetVersion: int64(block.ResetVersion), Sort: []ir.VisualizationSort{{Field: ref(block.Sort.Key), Direction: sortDirection(block.Sort.Direction)}}}
	}
	cardinality := ir.VisualizationCardinality{Kind: cardinalityKind(table.Cardinality.Kind)}
	if cardinality.Kind != ir.VisualizationCardinalityKindUnknown {
		count := int64(table.Cardinality.Value)
		cardinality.Count = &count
	}
	state := ir.WindowedVisualizationDataState{VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "windowed", SpecRevision: revision.String(), DataRevision: dataRevision, Generation: generation}, Kind: "windowed", Schema: schema, Cardinality: cardinality, AvailableRows: int64(table.AvailableRows), RowCap: maximumRows, ChunkSize: int64(max(table.ChunkSize, dashboard.TableChunkSize)), ResetVersion: int64(table.ResetVersion), Sort: []ir.VisualizationSort{sortValue}, Blocks: blocks}
	message := table.Error
	status := statusKind(table.AvailableRows, message)
	envelope := ir.VisualizationEnvelope{SchemaVersion: ir.CurrentSchemaVersion, VisualID: id, RendererID: "tanstack", SpecRevision: revision.String(), Spec: spec, DataRevision: dataRevision, DataState: ir.VisualizationDataState{Value: &state}, Selection: []ir.VisualizationSelectionEntry{}, Status: ir.VisualizationStatus{Kind: status, Message: optional(message)}, Diagnostics: []ir.VisualizationDiagnostic{}}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("visualization %q: %w", id, err)
	}
	return envelope, nil
}

func visualSpec(visual dashboard.Visual, base ir.VisualizationSpecBase, columns []string) (ir.VisualizationSpec, string, error) {
	field := func(candidates ...string) ir.VisualizationFieldRef {
		for _, candidate := range candidates {
			if contains(columns, candidate) {
				return ref(candidate)
			}
		}
		if len(columns) > 0 {
			return ref(columns[0])
		}
		return ref("value")
	}
	measureRefs := func(exclude ...string) []ir.VisualizationFieldRef {
		out := []ir.VisualizationFieldRef{}
		for _, column := range columns {
			if !contains(exclude, column) {
				out = append(out, ref(column))
			}
		}
		return out
	}
	base.Kind = "cartesian"
	common := ir.VisualizationPresentation{Legend: legendOption(visual.Options), ShowLabels: boolOption(visual.Options, "show_labels")}
	switch visual.Type {
	case "kpi":
		base.Kind = "kpi"
		return ir.VisualizationSpec{Value: &ir.KPIVisualizationSpec{VisualizationSpecBase: base, Kind: "kpi", Value: field("value"), Presentation: ir.KPIVisualizationPresentation{Trend: kpiTrend(visual.Options), Note: stringOption(visual.Options, "note"), Tone: toneOption(visual.Options), Thresholds: thresholdOptions(visual.Options)}}}, "html", nil
	case "pie", "donut", "funnel":
		base.Kind = "proportional"
		return ir.VisualizationSpec{Value: &ir.ProportionalVisualizationSpec{VisualizationSpecBase: base, Kind: "proportional", Mark: ir.VisualizationProportionalMark(visual.Type), Category: field("label"), Value: field("value"), Series: optionalRef(columns, "series"), Presentation: ir.ProportionalVisualizationPresentation{VisualizationPresentation: common, Orientation: orientationOption(visual.Options), Rose: stringValue(visual.Options["rose_type"]) != "", CenterLabel: stringOption(visual.Options, "center_label"), LabelPosition: labelPositionOption(visual.Options), InnerRadius: floatOption(visual.Options, "inner_radius"), OuterRadius: floatOption(visual.Options, "outer_radius"), Align: stringOption(visual.Options, "align"), Sort: proportionalSortOption(visual.Options)}}}, "echarts", nil
	case "treemap", "sunburst", "tree", "sankey", "graph":
		base.Kind = "hierarchy"
		spec := &ir.HierarchyVisualizationSpec{VisualizationSpecBase: base, Kind: "hierarchy", Mark: ir.VisualizationHierarchyMark(visual.Type), Node: field("node", "source", "label"), Value: optionalField(columns, "value"), Presentation: ir.HierarchyVisualizationPresentation{VisualizationPresentation: common, Orientation: orientationOption(visual.Options), InitialDepth: int32Option(visual.Options, "initial_depth"), Roam: boolOption(visual.Options, "roam"), Layout: hierarchyLayoutOption(visual.Options), Breadcrumb: boolPointerOption(visual.Options, "breadcrumb"), NodeGap: floatOption(visual.Options, "node_gap"), Curveness: floatOption(visual.Options, "curveness"), Focus: graphFocusOption(visual.Options)}}
		spec.Parent = optionalField(columns, "parent")
		spec.Source = optionalField(columns, "source")
		spec.Target = optionalField(columns, "target")
		return ir.VisualizationSpec{Value: spec}, "echarts", nil
	case "radar", "gauge":
		base.Kind = "polar"
		return ir.VisualizationSpec{Value: &ir.PolarVisualizationSpec{VisualizationSpecBase: base, Kind: "polar", Mark: ir.VisualizationPolarMark(visual.Type), Category: optionalRef(columns, "label"), Value: field("value"), Series: optionalRef(columns, "series"), Presentation: ir.PolarVisualizationPresentation{VisualizationPresentation: common, Minimum: floatOption(visual.Options, "min"), Maximum: floatOption(visual.Options, "max"), ShowPointer: !falseOption(visual.Options, "show_pointer"), Area: boolPointerOption(visual.Options, "area"), ProgressWidth: floatOption(visual.Options, "progress_width"), Thresholds: thresholdOptions(visual.Options)}}}, "echarts", nil
	case "map":
		base.Kind = "geographic"
		geometry, err := visualizationgeometry.Resolve("brazil_states")
		if err != nil {
			return ir.VisualizationSpec{}, "", err
		}
		join := field("name")
		layer := ir.VisualizationGeographicLayer{ID: "states", Kind: ir.VisualizationGeographicLayerKindChoropleth, Geometry: &geometry, Join: &join, Value: optionalField(columns, "value")}
		return ir.VisualizationSpec{Value: &ir.GeographicVisualizationSpec{VisualizationSpecBase: base, Kind: "geographic", Layers: []ir.VisualizationGeographicLayer{layer}, Presentation: ir.GeographicVisualizationPresentation{VisualizationPresentation: common, Roam: boolOption(visual.Options, "roam")}}}, "maplibre", nil
	default:
		mark := ir.VisualizationCartesianMark(visual.Type)
		supported := map[ir.VisualizationCartesianMark]bool{ir.VisualizationCartesianMarkLine: true, ir.VisualizationCartesianMarkArea: true, ir.VisualizationCartesianMarkBar: true, ir.VisualizationCartesianMarkColumn: true, ir.VisualizationCartesianMarkScatter: true, ir.VisualizationCartesianMarkHistogram: true, ir.VisualizationCartesianMarkCombo: true, ir.VisualizationCartesianMarkWaterfall: true, ir.VisualizationCartesianMarkCandlestick: true, ir.VisualizationCartesianMarkBoxplot: true, ir.VisualizationCartesianMarkHeatmap: true}
		if !supported[mark] {
			return ir.VisualizationSpec{}, "", fmt.Errorf("unsupported visualization type %q", visual.Type)
		}
		x := field("label", "row", "name")
		y := measureRefs(x.Field, "series", "selected", "positive")
		if len(y) == 0 {
			y = []ir.VisualizationFieldRef{field("value")}
		}
		presentation := ir.CartesianVisualizationPresentation{VisualizationPresentation: common, Smooth: boolOption(visual.Options, "smooth"), Stacked: boolOption(visual.Options, "stacked"), ShowSymbols: !falseOption(visual.Options, "show_symbols"), DataZoom: boolOption(visual.Options, "data_zoom"), Area: visual.Type == "area" || boolOption(visual.Options, "area"), Step: boolOption(visual.Options, "step"), Orientation: orientationPointerOption(visual.Options), LabelPosition: labelPositionOption(visual.Options), SymbolSize: floatOption(visual.Options, "symbol_size"), HistogramBins: int32Option(visual.Options, "bin_count"), ComboSeries: comboSeriesOptions(visual.Options)}
		return ir.VisualizationSpec{Value: &ir.CartesianVisualizationSpec{VisualizationSpecBase: base, Kind: "cartesian", Mark: mark, X: x, Y: y, Series: optionalRef(columns, "series"), Presentation: presentation}}, "echarts", nil
	}
}

func normalizedVisualData(shape string, values []dashboard.Datum) []dashboard.Datum {
	out := make([]dashboard.Datum, len(values))
	for index, value := range values {
		next := dashboard.Datum{}
		for key, item := range value {
			if key != "selected" {
				next[key] = item
			}
		}
		if shape == "hierarchy" {
			if path, ok := next["path"].([]string); ok && len(path) > 0 {
				next["node"] = strings.Join(path, "/")
				if len(path) > 1 {
					next["parent"] = strings.Join(path[:len(path)-1], "/")
				}
				delete(next, "path")
			}
		}
		out[index] = next
	}
	return out
}

func visualColumns(shape string, values []dashboard.Datum) []string {
	preferred := map[string][]string{"single_value": {"label", "value", "series"}, "category_value": {"label", "value"}, "category_series_value": {"label", "series", "value"}, "category_multi_measure": {"label", "series", "value"}, "category_delta": {"label", "value", "start", "end", "positive"}, "binned_measure": {"label", "binStart", "binEnd", "value"}, "hierarchy": {"node", "parent", "value"}, "matrix": {"row", "column", "value"}, "graph": {"source", "target", "value"}, "geo": {"name", "value"}, "ohlc": {"label", "open", "close", "low", "high"}, "distribution": {"label", "min", "q1", "median", "q3", "max"}}[shape]
	seen := map[string]bool{}
	out := []string{}
	for _, key := range preferred {
		if len(values) == 0 {
			out = append(out, key)
			seen[key] = true
			continue
		}
		for _, value := range values {
			if _, ok := value[key]; ok {
				out = append(out, key)
				seen[key] = true
				break
			}
		}
	}
	extras := []string{}
	for _, value := range values {
		for key := range value {
			if !seen[key] {
				seen[key] = true
				extras = append(extras, key)
			}
		}
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func schemaFromData(columns []string, values []dashboard.Datum) ir.VisualizationDatasetSchema {
	fields := make([]ir.VisualizationField, len(columns))
	for index, column := range columns {
		role := ir.VisualizationFieldRoleDimension
		if isMeasureColumn(column) {
			role = ir.VisualizationFieldRoleMeasure
		}
		fields[index] = ir.VisualizationField{ID: column, Role: role, DataType: inferType(column, values), Nullable: true, Label: title(column)}
	}
	return ir.VisualizationDatasetSchema{ID: primaryDataset, Fields: fields}
}
func inferType(column string, values []dashboard.Datum) ir.VisualizationDataType {
	if isMeasureColumn(column) {
		return ir.VisualizationDataTypeDecimal
	}
	// Derived shaping fields have a contractual type even when the compiler
	// builds the immutable specification before query data exists.
	if column == "positive" || column == "selected" {
		return ir.VisualizationDataTypeBoolean
	}
	for _, row := range values {
		switch row[column].(type) {
		case bool:
			return ir.VisualizationDataTypeBoolean
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return ir.VisualizationDataTypeInteger
		case float32, float64:
			return ir.VisualizationDataTypeDecimal
		}
	}
	return ir.VisualizationDataTypeString
}
func isMeasureColumn(value string) bool {
	return contains([]string{"value", "start", "end", "binStart", "binEnd", "open", "close", "low", "high", "min", "q1", "median", "q3", "max"}, value)
}
func interactions(visual dashboard.Visual, schema ir.VisualizationDatasetSchema) []ir.VisualizationInteraction {
	return runtimeInteractions(visual.Interaction, schema)
}

func runtimeInteractions(config dashboard.InteractionConfig, schema ir.VisualizationDatasetSchema) []ir.VisualizationInteraction {
	mappings := make([]ir.VisualizationInteractionMapping, 0, len(config.Mappings))
	for _, mapping := range config.Mappings {
		if containsField(schema, mapping.Value) {
			value := ir.VisualizationInteractionMapping{Source: ref(mapping.Value), TargetFieldID: mapping.Field, TargetFactID: optional(mapping.Fact), Grain: optional(mapping.Grain)}
			if mapping.Label != "" && containsField(schema, mapping.Label) {
				value.Label = optionalRef([]string{mapping.Label}, mapping.Label)
			}
			mappings = append(mappings, value)
		}
	}
	if len(mappings) == 0 {
		return []ir.VisualizationInteraction{}
	}
	mode := ir.VisualizationSelectionModeSingle
	if config.Toggle {
		mode = ir.VisualizationSelectionModeMultiple
	}
	interactionID := config.Kind
	if interactionID == "" || interactionID == "select" {
		interactionID = "point_selection"
	}
	return []ir.VisualizationInteraction{{ID: interactionID, Kind: ir.VisualizationInteractionKindSelect, Mappings: mappings, Targets: append([]string(nil), config.Targets...), Mode: mode, RequiresStableIdentity: true}}
}
func markIdentityFields(schema *ir.VisualizationDatasetSchema, config dashboard.InteractionConfig) {
	wanted := make(map[string]struct{}, len(config.Mappings))
	for _, mapping := range config.Mappings {
		wanted[mapping.Value] = struct{}{}
	}
	for index := range schema.Fields {
		if _, ok := wanted[schema.Fields[index].ID]; ok {
			schema.Fields[index].Role = ir.VisualizationFieldRoleIdentity
		}
	}
}
func row(columns []string, values map[string]any) []any {
	out := make([]any, len(columns))
	for index, column := range columns {
		out[index] = values[column]
	}
	return out
}
func ref(field string) ir.VisualizationFieldRef {
	return ir.VisualizationFieldRef{Dataset: primaryDataset, Field: field}
}
func optionalRef(columns []string, field string) *ir.VisualizationFieldRef {
	if !contains(columns, field) {
		return nil
	}
	value := ref(field)
	return &value
}
func optionalField(columns []string, field string) *ir.VisualizationFieldRef {
	return optionalRef(columns, field)
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func containsField(schema ir.VisualizationDatasetSchema, target string) bool {
	for _, field := range schema.Fields {
		if field.ID == target {
			return true
		}
	}
	return false
}
func completeness(rows [][]any) ir.VisualizationCompleteness {
	if len(rows) == 0 {
		return ir.VisualizationCompletenessEmpty
	}
	return ir.VisualizationCompletenessComplete
}
func statusKind(count int, message string) ir.VisualizationStatusKind {
	if message != "" {
		return ir.VisualizationStatusKindError
	}
	if count == 0 {
		return ir.VisualizationStatusKindNoData
	}
	return ir.VisualizationStatusKindReady
}
func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
func title(value string) string { return strings.Title(strings.ReplaceAll(value, "_", " ")) }
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func boolOption(options map[string]any, key string) bool {
	value, _ := options[key].(bool)
	return value
}
func falseOption(options map[string]any, key string) bool {
	value, ok := options[key].(bool)
	return ok && !value
}
func stringOption(options map[string]any, key string) *string {
	value, _ := options[key].(string)
	return optional(value)
}
func stringValue(value any) string { out, _ := value.(string); return out }
func floatOption(options map[string]any, key string) *float64 {
	switch value := options[key].(type) {
	case float64:
		return &value
	case int:
		out := float64(value)
		return &out
	}
	return nil
}
func int32Option(options map[string]any, key string) *int32 {
	switch value := options[key].(type) {
	case int:
		out := int32(value)
		return &out
	case int32:
		return &value
	case int64:
		out := int32(value)
		return &out
	case float64:
		out := int32(value)
		return &out
	}
	return nil
}
func boolPointerOption(options map[string]any, key string) *bool {
	value, ok := options[key].(bool)
	if !ok {
		return nil
	}
	return &value
}
func legendOption(options map[string]any) ir.VisualizationLegendPosition {
	switch stringValue(options["legend"]) {
	case "hidden":
		return ir.VisualizationLegendPositionHidden
	case "top":
		return ir.VisualizationLegendPositionTop
	case "right":
		return ir.VisualizationLegendPositionRight
	case "left":
		return ir.VisualizationLegendPositionLeft
	default:
		return ir.VisualizationLegendPositionBottom
	}
}
func orientationOption(options map[string]any) ir.VisualizationOrientation {
	if stringValue(options["orientation"]) == "horizontal" {
		return ir.VisualizationOrientationHorizontal
	}
	return ir.VisualizationOrientationVertical
}
func orientationPointerOption(options map[string]any) *ir.VisualizationOrientation {
	if stringValue(options["orientation"]) == "" {
		return nil
	}
	value := orientationOption(options)
	return &value
}
func labelPositionOption(options map[string]any) *ir.VisualizationLabelPosition {
	value := stringValue(options["label_position"])
	if value == "" {
		return nil
	}
	out := ir.VisualizationLabelPosition(value)
	return &out
}
func hierarchyLayoutOption(options map[string]any) *ir.VisualizationHierarchyLayout {
	value := stringValue(options["layout"])
	if value == "" {
		return nil
	}
	out := ir.VisualizationHierarchyLayout(value)
	return &out
}
func graphFocusOption(options map[string]any) *ir.VisualizationGraphFocus {
	value := stringValue(options["focus"])
	if value == "" {
		return nil
	}
	out := ir.VisualizationGraphFocus(value)
	return &out
}
func proportionalSortOption(options map[string]any) *ir.VisualizationSortDirection {
	value := stringValue(options["sort"])
	if value == "" {
		return nil
	}
	out := ir.VisualizationSortDirection(value)
	return &out
}
func toneOption(options map[string]any) *ir.VisualizationTone {
	value := stringValue(options["tone"])
	if value == "" {
		return nil
	}
	out := ir.VisualizationTone(value)
	return &out
}
func thresholdOptions(options map[string]any) *[]ir.VisualizationThreshold {
	values, ok := options["thresholds"].([]map[string]any)
	if !ok || len(values) == 0 {
		return nil
	}
	out := make([]ir.VisualizationThreshold, 0, len(values))
	for _, value := range values {
		threshold := floatOption(value, "value")
		tone := stringValue(value["tone"])
		if threshold == nil || tone == "" {
			continue
		}
		out = append(out, ir.VisualizationThreshold{Value: *threshold, Tone: ir.VisualizationTone(tone)})
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}
func comboSeriesOptions(options map[string]any) *[]ir.VisualizationComboSeries {
	values, ok := options["series_types"].(map[string]string)
	if !ok || len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ir.VisualizationComboSeries, len(keys))
	for index, key := range keys {
		axis := ir.VisualizationAxisPrimary
		if boolOption(options, "dual_axis") && index > 0 {
			axis = ir.VisualizationAxisSecondary
		}
		out[index] = ir.VisualizationComboSeries{SeriesValue: key, Mark: ir.VisualizationCartesianMark(values[key]), Axis: axis}
	}
	return &out
}
func kpiTrend(options map[string]any) ir.VisualizationKPITrend {
	switch stringValue(options["tone"]) {
	case "success", "positive":
		return ir.VisualizationKPITrendPositive
	case "danger", "negative":
		return ir.VisualizationKPITrendNegative
	default:
		return ir.VisualizationKPITrendNeutral
	}
}
func tableDataType(column dashboard.TableColumn, table dashboard.Table) ir.VisualizationDataType {
	switch column.Format {
	case "integer", "days":
		return ir.VisualizationDataTypeInteger
	case "decimal", "currency":
		return ir.VisualizationDataTypeDecimal
	case "boolean":
		return ir.VisualizationDataTypeBoolean
	case "date":
		return ir.VisualizationDataTypeDate
	case "timestamp":
		return ir.VisualizationDataTypeTemporal
	}
	for _, block := range table.Blocks {
		for _, row := range block.Rows {
			switch row[column.Key].(type) {
			case bool:
				return ir.VisualizationDataTypeBoolean
			case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
				return ir.VisualizationDataTypeInteger
			case float32, float64:
				return ir.VisualizationDataTypeDecimal
			case string:
				return ir.VisualizationDataTypeString
			}
		}
	}
	return ir.VisualizationDataTypeString
}
func tableFormat(column dashboard.TableColumn) *ir.VisualizationFormat {
	switch column.Format {
	case "integer", "decimal":
		value := ir.VisualizationFormat{Value: &ir.NumberVisualizationFormat{Kind: "number"}}
		return &value
	case "currency":
		value := ir.VisualizationFormat{Value: &ir.CurrencyVisualizationFormat{Kind: "currency", Currency: "BRL"}}
		return &value
	case "days":
		value := ir.VisualizationFormat{Value: &ir.DurationVisualizationFormat{Kind: "duration", Unit: "day"}}
		return &value
	case "date", "timestamp":
		value := ir.VisualizationFormat{Value: &ir.TemporalVisualizationFormat{Kind: "temporal"}}
		return &value
	}
	return nil
}

func tableFormatting(rules []dashboard.TableFormattingRule) []ir.TableVisualizationFormattingRule {
	out := make([]ir.TableVisualizationFormattingRule, 0, len(rules))
	for _, rule := range rules {
		switch rule.Kind {
		case "badge":
			out = append(out, ir.TableVisualizationFormattingRule{Value: &ir.TableBadgeFormattingRule{Kind: rule.Kind, Values: cloneStringMap(rule.Values)}})
		case "text_color":
			values := cloneStringMap(rule.Values)
			var valuesPointer *map[string]string
			if len(values) > 0 {
				valuesPointer = &values
			}
			out = append(out, ir.TableVisualizationFormattingRule{Value: &ir.TableTextColorFormattingRule{Kind: rule.Kind, Color: rule.Color, Values: valuesPointer, Minimum: rule.Min, Maximum: rule.Max}})
		case "background_scale":
			out = append(out, ir.TableVisualizationFormattingRule{Value: &ir.TableBackgroundScaleFormattingRule{Kind: rule.Kind, Minimum: rule.Min, Maximum: rule.Max, LowColor: optional(rule.LowColor), HighColor: optional(rule.HighColor)}})
		case "data_bar":
			out = append(out, ir.TableVisualizationFormattingRule{Value: &ir.TableDataBarFormattingRule{Kind: rule.Kind, Minimum: rule.Min, Maximum: rule.Max, Color: rule.Color, Background: optional(rule.Background)}})
		}
	}
	return out
}

// TableFormatting compiles authoring table rules into the closed IR union.
func TableFormatting(rules []dashboard.TableFormattingRule) []ir.TableVisualizationFormattingRule {
	return tableFormatting(rules)
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
func sortDirection(value string) ir.VisualizationSortDirection {
	if value == "desc" {
		return ir.VisualizationSortDirectionDescending
	}
	return ir.VisualizationSortDirectionAscending
}
func cardinalityKind(value string) ir.VisualizationCardinalityKind {
	switch value {
	case dashboard.CardinalityExact:
		return ir.VisualizationCardinalityKindExact
	case dashboard.CardinalityEstimated:
		return ir.VisualizationCardinalityKindEstimated
	case dashboard.CardinalityLowerBound:
		return ir.VisualizationCardinalityKindLowerBound
	default:
		return ir.VisualizationCardinalityKindUnknown
	}
}
func gridRefs(fields []ir.VisualizationField) ([]ir.VisualizationFieldRef, []ir.VisualizationFieldRef, []ir.VisualizationFieldRef) {
	dimensions, measures := []ir.VisualizationFieldRef{}, []ir.VisualizationFieldRef{}
	for _, field := range fields {
		if field.Role == ir.VisualizationFieldRoleMeasure {
			measures = append(measures, ref(field.ID))
		} else {
			dimensions = append(dimensions, ref(field.ID))
		}
	}
	rows := dimensions
	columns := []ir.VisualizationFieldRef{}
	if len(dimensions) > 1 {
		rows, columns = dimensions[:1], dimensions[1:]
	}
	return rows, columns, measures
}
