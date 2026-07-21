// Package runtime shapes governed query results into the visualization IR.
package runtime

import (
	"fmt"
	"strings"

	"github.com/Yacobolo/libredash/internal/dashboard"
	visualizationdefinition "github.com/Yacobolo/libredash/internal/visualization/definition"
	"github.com/Yacobolo/libredash/internal/visualization/ir"
)

const primaryDataset = "primary"

// Frame is the renderer-independent result of a compiled visualization query.
// Columns must use compiled field aliases; rows are ordered to those columns.
type Frame struct {
	Columns []string
	Rows    [][]any
}

// FrameFromRecords orders named query values according to the immutable
// compiled dataset schema. It is the shared boundary for non-dashboard
// producers such as agent-generated visualizations.
func FrameFromRecords(definition visualizationdefinition.Definition, records []map[string]any) (Frame, error) {
	base, err := ir.SpecificationBase(definition.Spec)
	if err != nil {
		return Frame{}, err
	}
	schema, err := compiledDatasetSchema(base, definition.Query.DatasetID)
	if err != nil {
		return Frame{}, err
	}
	columns := make([]string, len(schema.Fields))
	for index, field := range schema.Fields {
		columns[index] = field.ID
	}
	rows := make([][]any, len(records))
	for rowIndex, record := range records {
		rows[rowIndex] = make([]any, len(columns))
		for columnIndex, column := range columns {
			rows[rowIndex][columnIndex] = record[column]
		}
	}
	return Frame{Columns: columns, Rows: rows}, nil
}

// SelectionEntriesFromDefinition projects canonical dashboard selection state
// into renderer-independent DatumRef values.
func SelectionEntriesFromDefinition(definition visualizationdefinition.Definition, entries []dashboard.InteractionSelectionEntry, dataRevision int64) ([]ir.VisualizationSelectionEntry, error) {
	return compiledSelections(definition.Spec, entries, dataRevision)
}

// EnvelopeFromFrame creates the canonical inline renderer boundary directly
// from a compiled query frame. No legacy visual presentation DTO participates
// in this path.
func EnvelopeFromFrame(definition visualizationdefinition.Definition, frame Frame, selections []dashboard.InteractionSelectionEntry, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
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
	wantColumns := make([]string, len(schema.Fields))
	for index, field := range schema.Fields {
		wantColumns[index] = field.ID
	}
	if err := validateFrameColumns(definition.ID, frame.Columns, wantColumns); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	state := ir.InlineVisualizationDataState{
		VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "inline", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
		Kind:                       "inline", Datasets: []ir.VisualizationInlineDataset{{
			ID: definition.Query.DatasetID, SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation,
			Columns: append([]string{}, frame.Columns...), Rows: frame.Rows, Completeness: completeness(frame.Rows),
		}},
	}
	envelope := ir.VisualizationEnvelope{
		SchemaVersion: ir.CurrentSchemaVersion, VisualID: definition.ID, RendererID: definition.RendererID, SpecRevision: definition.SpecRevision, Spec: definition.Spec,
		DataRevision: dataRevision, DataState: ir.VisualizationDataState{Value: &state}, Status: ir.VisualizationStatus{Kind: statusKind(len(frame.Rows), "")}, Diagnostics: []ir.VisualizationDiagnostic{},
	}
	envelope.Selection, err = compiledSelections(definition.Spec, selections, dataRevision)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled visualization %q: %w", definition.ID, err)
	}
	return envelope, nil
}

func validateFrameColumns(visualID string, got, want []string) error {
	if len(got) != len(want) {
		return fmt.Errorf("visualization %q frame has %d columns, want %d", visualID, len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("visualization %q frame column %d is %q, want %q", visualID, index, got[index], want[index])
		}
	}
	return nil
}

// SpatialEnvelopeFromFrame packages an already governed and bounded spatial
// query result. Spatial aggregation belongs to the database planner; this
// boundary validates and serializes it without filtering or re-aggregating.
func SpatialEnvelopeFromFrame(definition visualizationdefinition.Definition, frame Frame, selections []dashboard.InteractionSelectionEntry, request dashboard.SpatialWindowRequest, precision ir.VisualizationSpatialPrecision, cardinality int64, dataRevision, generation int64) (ir.VisualizationEnvelope, error) {
	if err := definition.Validate(); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	geographic, ok := definition.Spec.Value.(*ir.GeographicVisualizationSpec)
	if !ok || definition.Query.Kind != visualizationdefinition.QuerySpatial || definition.Query.Spatial == nil || definition.Query.Spatial.Viewport == nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("visualization %q has no compiled spatial viewport", definition.ID)
	}
	schema, err := compiledDatasetSchema(geographic.VisualizationSpecBase, definition.Query.DatasetID)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	columns := make([]string, len(schema.Fields))
	for index, field := range schema.Fields {
		columns[index] = field.ID
	}
	if err := validateFrameColumns(definition.ID, frame.Columns, columns); err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	featureCap := definition.Query.Spatial.Viewport.FeatureCap
	state := ir.SpatialWindowedVisualizationDataState{
		VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "spatial_windowed", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
		Kind:                       "spatial_windowed", Schema: schema, Cardinality: ir.VisualizationCardinality{Kind: ir.VisualizationCardinalityKindExact, Count: &cardinality},
		Extent: spatialBounds(request.Bounds), RowCap: definition.Query.Spatial.Limit, FeatureCap: featureCap, ResetVersion: request.ResetVersion,
		Window: &ir.VisualizationSpatialWindowBlock{ID: request.WindowID, Bounds: spatialBounds(request.Bounds), Zoom: request.Zoom, Width: int32(request.Width), Height: int32(request.Height), Precision: precision, Rows: frame.Rows, RequestSeq: request.RequestSeq, ResetVersion: request.ResetVersion},
	}
	status := ir.VisualizationStatusKindReady
	if len(frame.Rows) == 0 {
		status = ir.VisualizationStatusKindNoData
	} else if precision == ir.VisualizationSpatialPrecisionAggregated {
		status = ir.VisualizationStatusKindPartial
	}
	envelope := ir.VisualizationEnvelope{
		SchemaVersion: ir.CurrentSchemaVersion, VisualID: definition.ID, RendererID: definition.RendererID, SpecRevision: definition.SpecRevision, Spec: definition.Spec,
		DataRevision: dataRevision, DataState: ir.VisualizationDataState{Value: &state}, Status: ir.VisualizationStatus{Kind: status}, Diagnostics: []ir.VisualizationDiagnostic{},
	}
	envelope.Selection, err = compiledSelections(definition.Spec, selections, dataRevision)
	if err != nil {
		return ir.VisualizationEnvelope{}, err
	}
	if err := ir.ValidateEnvelope(envelope); err != nil {
		return ir.VisualizationEnvelope{}, fmt.Errorf("compiled spatial visualization %q: %w", definition.ID, err)
	}
	return envelope, nil
}

func spatialBounds(value dashboard.SpatialBounds) ir.VisualizationSpatialBounds {
	return ir.VisualizationSpatialBounds{West: value.West, South: value.South, East: value.East, North: value.North}
}

func compiledDatasetSchema(base ir.VisualizationSpecBase, datasetID string) (ir.VisualizationDatasetSchema, error) {
	for _, schema := range base.Datasets {
		if schema.ID == datasetID {
			return schema, nil
		}
	}
	return ir.VisualizationDatasetSchema{}, fmt.Errorf("query targets unknown dataset %q", datasetID)
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
	if definition.Query.Kind == visualizationdefinition.QuerySpatial && definition.Query.Spatial != nil && definition.Query.Spatial.Viewport != nil {
		geographic, ok := definition.Spec.Value.(*ir.GeographicVisualizationSpec)
		if !ok {
			return ir.VisualizationEnvelope{}, fmt.Errorf("compiled spatial visualization %q is not geographic", definition.ID)
		}
		extent := ir.VisualizationSpatialBounds{West: -180, South: -85, East: 180, North: 85}
		if asset := geographic.Presentation.Basemap; asset != nil && len(asset.Bounds) == 4 {
			extent = ir.VisualizationSpatialBounds{West: asset.Bounds[0], South: asset.Bounds[1], East: asset.Bounds[2], North: asset.Bounds[3]}
		}
		state := ir.SpatialWindowedVisualizationDataState{
			VisualizationDataStateBase: ir.VisualizationDataStateBase{Kind: "spatial_windowed", SpecRevision: definition.SpecRevision, DataRevision: dataRevision, Generation: generation},
			Kind:                       "spatial_windowed", Schema: schema, Cardinality: ir.VisualizationCardinality{Kind: ir.VisualizationCardinalityKindUnknown}, Extent: extent,
			RowCap: base.DataBudget.MaxRows, FeatureCap: 5000, ResetVersion: resetVersion,
		}
		envelope.DataState = ir.VisualizationDataState{Value: &state}
	} else if definition.Query.Kind == visualizationdefinition.QueryDetail || definition.Query.Kind == visualizationdefinition.QueryMatrix || definition.Query.Kind == visualizationdefinition.QueryPivot {
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
func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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
