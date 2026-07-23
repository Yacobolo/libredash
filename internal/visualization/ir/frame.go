package ir

import (
	"fmt"
	"math"
	"strings"
)

// ValidateEnvelope validates the complete renderer boundary: immutable
// specification identity, frame shape and scalar types, and window invariants.
func ValidateEnvelope(envelope VisualizationEnvelope) error {
	if err := ValidateEnvelopeRevisions(envelope); err != nil {
		return err
	}
	base, err := specificationBase(envelope.Spec)
	if err != nil {
		return err
	}
	if envelope.VisualID == "" || envelope.RendererID == "" {
		return fmt.Errorf("visualization ID and renderer ID are required")
	}
	if base.DataBudget.MaxRows <= 0 {
		return fmt.Errorf("visualization data budget maxRows must be positive")
	}
	schemas, err := validateSpecification(envelope.Spec, base)
	if err != nil {
		return err
	}
	if err := validateSelections(envelope, schemas); err != nil {
		return err
	}
	switch state := envelope.DataState.Value.(type) {
	case *InlineVisualizationDataState:
		if err := validateInlineState(*state, schemas, base.DataBudget); err != nil {
			return err
		}
		return validateInlineSemantics(envelope.Spec, *state)
	case *WindowedVisualizationDataState:
		return validateWindowedState(*state, base.DataBudget)
	case *SpatialWindowedVisualizationDataState:
		return validateSpatialWindowedState(*state, base.DataBudget)
	default:
		return fmt.Errorf("unsupported visualization data state %T", state)
	}
}

func validateInlineSemantics(spec VisualizationSpec, state InlineVisualizationDataState) error {
	hierarchy, ok := spec.Value.(*HierarchyVisualizationSpec)
	if !ok {
		return nil
	}
	if hierarchy.Mark == VisualizationHierarchyMarkGraph || hierarchy.Mark == VisualizationHierarchyMarkSankey {
		return validateNetworkRows(*hierarchy, state)
	}
	return validateHierarchyRows(*hierarchy, state)
}

func validateHierarchyRows(spec HierarchyVisualizationSpec, state InlineVisualizationDataState) error {
	if spec.Parent == nil || spec.Parent.Dataset != spec.Node.Dataset {
		return fmt.Errorf("hierarchy node and parent fields must share a dataset")
	}
	dataset, ok := inlineDataset(state, spec.Node.Dataset)
	if !ok {
		return fmt.Errorf("hierarchy dataset %q is missing", spec.Node.Dataset)
	}
	nodeIndex, parentIndex := columnIndex(dataset.Columns, spec.Node.Field), columnIndex(dataset.Columns, spec.Parent.Field)
	if nodeIndex < 0 || parentIndex < 0 {
		return fmt.Errorf("hierarchy node or parent column is missing")
	}
	parents := make(map[string]string, len(dataset.Rows))
	for rowIndex, row := range dataset.Rows {
		node, ok := row[nodeIndex].(string)
		if !ok || strings.TrimSpace(node) == "" {
			return fmt.Errorf("hierarchy row %d has an empty node", rowIndex)
		}
		parent := ""
		if row[parentIndex] != nil {
			var parentOK bool
			parent, parentOK = row[parentIndex].(string)
			if !parentOK || strings.TrimSpace(parent) == "" {
				return fmt.Errorf("hierarchy row %d has an invalid parent", rowIndex)
			}
		}
		id := HierarchyNodeIdentity(parent, node)
		if _, exists := parents[id]; exists {
			return fmt.Errorf("duplicate hierarchy node identity %q", id)
		}
		parents[id] = parent
	}
	for id, parent := range parents {
		if parent != "" {
			if _, exists := parents[parent]; !exists {
				return fmt.Errorf("hierarchy node %q references missing parent %q", id, parent)
			}
		}
	}
	for id := range parents {
		seen := map[string]struct{}{id: {}}
		for parent := parents[id]; parent != ""; parent = parents[parent] {
			if _, exists := seen[parent]; exists {
				return fmt.Errorf("hierarchy contains a cycle at %q", parent)
			}
			seen[parent] = struct{}{}
		}
	}
	return nil
}

func validateNetworkRows(spec HierarchyVisualizationSpec, state InlineVisualizationDataState) error {
	if spec.Source == nil || spec.Target == nil || spec.Source.Dataset != spec.Target.Dataset {
		return fmt.Errorf("network source and target fields must share a dataset")
	}
	dataset, ok := inlineDataset(state, spec.Source.Dataset)
	if !ok {
		return fmt.Errorf("network dataset %q is missing", spec.Source.Dataset)
	}
	sourceIndex, targetIndex := columnIndex(dataset.Columns, spec.Source.Field), columnIndex(dataset.Columns, spec.Target.Field)
	if sourceIndex < 0 || targetIndex < 0 {
		return fmt.Errorf("network source or target column is missing")
	}
	for rowIndex, row := range dataset.Rows {
		for _, endpoint := range []struct {
			name  string
			value any
		}{{"source", row[sourceIndex]}, {"target", row[targetIndex]}} {
			value, ok := endpoint.value.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("network row %d has an invalid %s endpoint", rowIndex, endpoint.name)
			}
		}
	}
	return nil
}

func inlineDataset(state InlineVisualizationDataState, id string) (VisualizationInlineDataset, bool) {
	for _, dataset := range state.Datasets {
		if dataset.ID == id {
			return dataset, true
		}
	}
	return VisualizationInlineDataset{}, false
}

func columnIndex(columns []string, id string) int {
	for index, column := range columns {
		if column == id {
			return index
		}
	}
	return -1
}

// HierarchyNodeIdentity returns the canonical identity used by frame builders,
// validators, and renderer adapters. Parent is already a canonical identity;
// node remains the author-facing display label.
func HierarchyNodeIdentity(parent, node string) string {
	escaped := strings.ReplaceAll(node, "\x1f", "\x1f\x1f")
	if parent == "" {
		return escaped
	}
	return parent + "\x1f" + escaped
}

func validateSelections(envelope VisualizationEnvelope, schemas map[string]VisualizationDatasetSchema) error {
	for index, selection := range envelope.Selection {
		datum := selection.Datum
		schema, ok := schemas[datum.Dataset]
		if !ok {
			return fmt.Errorf("selection %d references unknown dataset %q", index, datum.Dataset)
		}
		if datum.DataRevision != envelope.DataRevision {
			return fmt.Errorf("selection %d data revision mismatch", index)
		}
		if len(datum.Identity) == 0 {
			return fmt.Errorf("selection %d requires identity values", index)
		}
		identityFields := map[string]VisualizationField{}
		for _, field := range schema.Fields {
			if field.Role == VisualizationFieldRoleIdentity {
				identityFields[field.ID] = field
			}
		}
		if len(identityFields) == 0 {
			return fmt.Errorf("selection %d dataset has no identity fields", index)
		}
		for fieldID, value := range datum.Identity {
			field, ok := identityFields[fieldID]
			if !ok {
				return fmt.Errorf("selection %d references non-identity field %q", index, fieldID)
			}
			if err := validateScalar(field, value); err != nil {
				return fmt.Errorf("selection %d identity: %w", index, err)
			}
		}
		for fieldID := range identityFields {
			if _, ok := datum.Identity[fieldID]; !ok {
				return fmt.Errorf("selection %d omits identity field %q", index, fieldID)
			}
		}
	}
	return nil
}

// ValidateSpec validates semantic field references and data requirements
// without requiring a runtime data state.
func ValidateSpec(spec VisualizationSpec) error {
	base, err := specificationBase(spec)
	if err != nil {
		return err
	}
	_, err = validateSpecification(spec, base)
	return err
}

func specificationBase(spec VisualizationSpec) (VisualizationSpecBase, error) {
	base, err := spec.Base()
	if err != nil {
		return VisualizationSpecBase{}, err
	}
	return *base, nil
}

// SpecificationBase returns the common, renderer-independent contract shared
// by every closed visualization discriminator.
func SpecificationBase(spec VisualizationSpec) (VisualizationSpecBase, error) {
	return specificationBase(spec)
}

func validateSpecification(spec VisualizationSpec, base VisualizationSpecBase) (map[string]VisualizationDatasetSchema, error) {
	if base.Title == "" || base.Accessibility.Title == "" || base.Accessibility.Description == "" {
		return nil, fmt.Errorf("visualization title and accessibility text are required")
	}
	schemas := make(map[string]VisualizationDatasetSchema, len(base.Datasets))
	for _, schema := range base.Datasets {
		if err := validateSchema(schema); err != nil {
			return nil, err
		}
		if _, exists := schemas[schema.ID]; exists {
			return nil, fmt.Errorf("duplicate visualization dataset %q", schema.ID)
		}
		schemas[schema.ID] = schema
	}
	if len(schemas) == 0 {
		return nil, fmt.Errorf("visualization requires at least one dataset")
	}
	for _, ref := range specificationRefs(spec) {
		if err := validateFieldRef(ref, schemas); err != nil {
			return nil, err
		}
	}
	if err := validateGeographicSpecification(spec); err != nil {
		return nil, err
	}
	for _, interaction := range base.Interactions {
		if interaction.ID == "" {
			return nil, fmt.Errorf("visualization interaction ID is required")
		}
		if len(interaction.Mappings) == 0 {
			return nil, fmt.Errorf("interaction %q requires mappings", interaction.ID)
		}
		for _, mapping := range interaction.Mappings {
			if mapping.TargetFieldID == "" {
				return nil, fmt.Errorf("interaction %q mapping requires target field ID", interaction.ID)
			}
			if err := validateFieldRef(mapping.Source, schemas); err != nil {
				return nil, fmt.Errorf("interaction %q: %w", interaction.ID, err)
			}
			if mapping.Label != nil {
				if err := validateFieldRef(*mapping.Label, schemas); err != nil {
					return nil, fmt.Errorf("interaction %q label: %w", interaction.ID, err)
				}
			}
			if interaction.RequiresStableIdentity && !hasIdentityField(schemas[mapping.Source.Dataset]) {
				return nil, fmt.Errorf("interaction %q requires a stable identity field", interaction.ID)
			}
		}
	}
	return schemas, nil
}

func specificationRefs(spec VisualizationSpec) []VisualizationFieldRef {
	visitor := &specificationReferenceVisitor{refs: make([]VisualizationFieldRef, 0, 8)}
	if err := spec.Visit(visitor); err != nil {
		return nil
	}
	return visitor.refs
}

type specificationReferenceVisitor struct {
	refs []VisualizationFieldRef
}

func (visitor *specificationReferenceVisitor) add(ref *VisualizationFieldRef) {
	if ref != nil {
		visitor.refs = append(visitor.refs, *ref)
	}
}

func (visitor *specificationReferenceVisitor) VisitCartesianVisualizationSpec(value *CartesianVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.X)
	visitor.refs = append(visitor.refs, value.Y...)
	visitor.add(value.Series)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitProportionalVisualizationSpec(value *ProportionalVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.Category, value.Value)
	visitor.add(value.Series)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitHierarchyVisualizationSpec(value *HierarchyVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.Node)
	visitor.add(value.Parent)
	visitor.add(value.Source)
	visitor.add(value.Target)
	visitor.add(value.Value)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitPolarVisualizationSpec(value *PolarVisualizationSpec) error {
	visitor.add(value.Category)
	visitor.refs = append(visitor.refs, value.Value)
	visitor.add(value.Series)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitTableVisualizationSpec(value *TableVisualizationSpec) error {
	for _, column := range value.Columns {
		visitor.refs = append(visitor.refs, column.Field)
	}
	if value.DefaultSort != nil {
		for _, sort := range *value.DefaultSort {
			visitor.refs = append(visitor.refs, sort.Field)
		}
	}
	return nil
}

func (visitor *specificationReferenceVisitor) VisitMatrixVisualizationSpec(value *MatrixVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.Rows...)
	visitor.refs = append(visitor.refs, value.Columns...)
	visitor.refs = append(visitor.refs, value.Measures...)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitPivotVisualizationSpec(value *PivotVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.Rows...)
	visitor.refs = append(visitor.refs, value.Columns...)
	visitor.refs = append(visitor.refs, value.Measures...)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitKPIVisualizationSpec(value *KPIVisualizationSpec) error {
	visitor.refs = append(visitor.refs, value.Value)
	visitor.add(value.Comparison)
	visitor.add(value.Trend)
	return nil
}

func (visitor *specificationReferenceVisitor) VisitGeographicVisualizationSpec(value *GeographicVisualizationSpec) error {
	for _, layer := range value.Layers {
		base, err := layer.Base()
		if err == nil {
			visitor.add(base.Label)
			visitor.refs = append(visitor.refs, base.Tooltip...)
		}
		switch layer := layer.Value.(type) {
		case *VisualizationPointLayer:
			visitor.refs = append(visitor.refs, layer.Latitude, layer.Longitude)
			visitor.add(layer.Value)
			visitor.add(layer.Category)
		case *VisualizationChoroplethLayer:
			visitor.refs = append(visitor.refs, layer.Join)
			visitor.add(layer.Value)
			visitor.add(layer.Category)
		case *VisualizationHeatLayer:
			visitor.refs = append(visitor.refs, layer.Latitude, layer.Longitude)
			visitor.add(layer.Value)
		case *VisualizationDensityLayer:
			visitor.refs = append(visitor.refs, layer.Latitude, layer.Longitude)
			visitor.add(layer.Value)
		case *VisualizationPathLayer:
			visitor.refs = append(visitor.refs, layer.Latitude, layer.Longitude, layer.Path, layer.Order)
			visitor.add(layer.Value)
			visitor.add(layer.Category)
		}
	}
	return nil
}

func (visitor *specificationReferenceVisitor) VisitCustomVisualizationSpec(*CustomVisualizationSpec) error {
	return nil
}

func validateGeographicSpecification(spec VisualizationSpec) error {
	value, ok := spec.Value.(*GeographicVisualizationSpec)
	if !ok {
		return nil
	}
	if len(value.Layers) == 0 {
		return fmt.Errorf("geographic visualization requires at least one layer")
	}
	seen := map[string]struct{}{}
	for _, layer := range value.Layers {
		base, err := layer.Base()
		if err != nil {
			return err
		}
		if base.ID == "" {
			return fmt.Errorf("geographic layer ID is required")
		}
		if _, exists := seen[base.ID]; exists {
			return fmt.Errorf("duplicate geographic layer %q", base.ID)
		}
		seen[base.ID] = struct{}{}
		if base.Visibility.MinimumZoom < 0 || base.Visibility.MaximumZoom <= base.Visibility.MinimumZoom {
			return fmt.Errorf("geographic layer %q has invalid visibility", base.ID)
		}
		switch typed := layer.Value.(type) {
		case *VisualizationChoroplethLayer:
			if err := validateGeometryAsset(typed.Geometry); err != nil {
				return fmt.Errorf("choropleth layer %q: %w", base.ID, err)
			}
		case *VisualizationReferenceLayer:
			if err := validateGeometryAsset(typed.Geometry); err != nil {
				return fmt.Errorf("reference layer %q: %w", base.ID, err)
			}
		case *VisualizationPointLayer:
			if typed.Size.MinimumRadius < 0 || typed.Size.MaximumRadius < typed.Size.MinimumRadius {
				return fmt.Errorf("point layer %q has invalid size scale", base.ID)
			}
			if typed.Cluster.Radius <= 0 || typed.Cluster.MinimumPoints < 2 {
				return fmt.Errorf("point layer %q has invalid cluster configuration", base.ID)
			}
		case *VisualizationHeatLayer, *VisualizationDensityLayer, *VisualizationPathLayer:
		default:
			kind, _ := layer.Kind()
			return fmt.Errorf("unsupported geographic layer kind %q", kind)
		}
	}
	if value.Presentation.Basemap != nil {
		asset := value.Presentation.Basemap
		if asset.ID == "" || asset.StyleURL == "" || asset.ArchiveURL == "" || len(asset.StyleDigest) != 71 || len(asset.ArchiveDigest) != 71 || asset.Attribution == "" {
			return fmt.Errorf("geographic basemap has incomplete provenance")
		}
	}
	return nil
}

func validateGeometryAsset(geometry VisualizationGeometryAsset) error {
	if geometry.ID == "" || geometry.Source == "" || geometry.License == "" || geometry.Attribution == "" || geometry.IdentifierSystem == "" || geometry.URL == "" || len(geometry.Digest) != 71 || geometry.Digest[:7] != "sha256:" {
		return fmt.Errorf("incomplete geometry provenance")
	}
	return nil
}

func validateSchema(schema VisualizationDatasetSchema) error {
	if schema.ID == "" || len(schema.Fields) == 0 {
		return fmt.Errorf("visualization dataset ID and fields are required")
	}
	seen := make(map[string]struct{}, len(schema.Fields))
	for _, field := range schema.Fields {
		if field.ID == "" || field.Label == "" {
			return fmt.Errorf("visualization dataset %q has a field without ID or label", schema.ID)
		}
		if _, exists := seen[field.ID]; exists {
			return fmt.Errorf("visualization dataset %q has duplicate field %q", schema.ID, field.ID)
		}
		seen[field.ID] = struct{}{}
	}
	return nil
}

func validateFieldRef(ref VisualizationFieldRef, schemas map[string]VisualizationDatasetSchema) error {
	schema, ok := schemas[ref.Dataset]
	if !ok {
		return fmt.Errorf("unknown visualization dataset %q", ref.Dataset)
	}
	for _, field := range schema.Fields {
		if field.ID == ref.Field {
			return nil
		}
	}
	return fmt.Errorf("unknown visualization field %q in dataset %q", ref.Field, ref.Dataset)
}

func hasIdentityField(schema VisualizationDatasetSchema) bool {
	for _, field := range schema.Fields {
		if field.Role == VisualizationFieldRoleIdentity {
			return true
		}
	}
	return false
}

func validateInlineState(state InlineVisualizationDataState, schemas map[string]VisualizationDatasetSchema, budget VisualizationDataBudget) error {
	seen := make(map[string]struct{}, len(state.Datasets))
	for _, dataset := range state.Datasets {
		schema, ok := schemas[dataset.ID]
		if !ok {
			return fmt.Errorf("inline data targets unknown dataset %q", dataset.ID)
		}
		if _, exists := seen[dataset.ID]; exists {
			return fmt.Errorf("duplicate inline dataset %q", dataset.ID)
		}
		seen[dataset.ID] = struct{}{}
		if int64(len(dataset.Rows)) > budget.MaxRows {
			return fmt.Errorf("dataset %q exceeds row budget %d", dataset.ID, budget.MaxRows)
		}
		if budget.RequiredCompleteness == VisualizationCompletenessComplete && dataset.Completeness != VisualizationCompletenessComplete && dataset.Completeness != VisualizationCompletenessEmpty {
			return fmt.Errorf("dataset %q does not satisfy complete data requirement", dataset.ID)
		}
		if err := validateRows(schema, dataset.Columns, dataset.Rows); err != nil {
			return fmt.Errorf("dataset %q: %w", dataset.ID, err)
		}
	}
	return nil
}

func validateWindowedState(state WindowedVisualizationDataState, budget VisualizationDataBudget) error {
	if err := validateSchema(state.Schema); err != nil {
		return err
	}
	if state.AvailableRows < 0 || state.RowCap <= 0 || state.ChunkSize <= 0 || state.ResetVersion < 0 {
		return fmt.Errorf("invalid window bounds")
	}
	if state.RowCap > budget.MaxRows {
		return fmt.Errorf("window row cap %d exceeds budget %d", state.RowCap, budget.MaxRows)
	}
	switch state.Cardinality.Kind {
	case VisualizationCardinalityKindUnknown:
		if state.Cardinality.Count != nil {
			return fmt.Errorf("unknown window cardinality must omit count")
		}
	case VisualizationCardinalityKindExact:
		if state.Cardinality.Count == nil || *state.Cardinality.Count < state.AvailableRows {
			return fmt.Errorf("exact window cardinality is missing or smaller than available rows")
		}
	case VisualizationCardinalityKindLowerBound, VisualizationCardinalityKindEstimated:
		if state.Cardinality.Count == nil || *state.Cardinality.Count < 0 {
			return fmt.Errorf("window cardinality estimate is missing or negative")
		}
	default:
		return fmt.Errorf("unsupported window cardinality kind %q", state.Cardinality.Kind)
	}
	columns := make([]string, len(state.Schema.Fields))
	for index, field := range state.Schema.Fields {
		columns[index] = field.ID
	}
	for key, block := range state.Blocks {
		if key != block.ID || block.ID == "" {
			return fmt.Errorf("window block identity mismatch for %q", key)
		}
		if block.Start < 0 || block.RequestSeq < 0 || block.ResetVersion != state.ResetVersion {
			return fmt.Errorf("window block %q has stale or invalid coordinates", key)
		}
		if block.Start+int64(len(block.Rows)) > state.AvailableRows {
			return fmt.Errorf("window block %q exceeds available rows", key)
		}
		if err := validateRows(state.Schema, columns, block.Rows); err != nil {
			return fmt.Errorf("window block %q: %w", key, err)
		}
	}
	return nil
}

func validateSpatialWindowedState(state SpatialWindowedVisualizationDataState, budget VisualizationDataBudget) error {
	if err := validateSchema(state.Schema); err != nil {
		return err
	}
	if state.RowCap <= 0 || state.RowCap > budget.MaxRows || state.FeatureCap <= 0 || state.FeatureCap > 5000 || state.ResetVersion < 0 {
		return fmt.Errorf("invalid spatial window budgets")
	}
	if err := validateSpatialBounds(state.Extent); err != nil {
		return fmt.Errorf("invalid spatial extent: %w", err)
	}
	if state.Window == nil {
		return nil
	}
	window := state.Window
	if window.ID == "" || window.RequestSeq <= 0 || window.ResetVersion != state.ResetVersion || window.Width <= 0 || window.Width > 16384 || window.Height <= 0 || window.Height > 16384 || window.Zoom < 0 || window.Zoom > 24 || int64(len(window.Rows)) > state.FeatureCap {
		return fmt.Errorf("invalid or stale spatial window")
	}
	if err := validateSpatialBounds(window.Bounds); err != nil {
		return fmt.Errorf("invalid spatial window bounds: %w", err)
	}
	if window.Precision != VisualizationSpatialPrecisionRaw && window.Precision != VisualizationSpatialPrecisionAggregated {
		return fmt.Errorf("unsupported spatial precision %q", window.Precision)
	}
	columns := make([]string, len(state.Schema.Fields))
	for index, field := range state.Schema.Fields {
		columns[index] = field.ID
	}
	if err := validateRows(state.Schema, columns, window.Rows); err != nil {
		return fmt.Errorf("spatial window %q: %w", window.ID, err)
	}
	return nil
}

func validateSpatialBounds(bounds VisualizationSpatialBounds) error {
	for _, coordinate := range []float64{bounds.West, bounds.South, bounds.East, bounds.North} {
		if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return fmt.Errorf("coordinates must be finite")
		}
	}
	if bounds.West < -180 || bounds.West > 180 || bounds.East < -180 || bounds.East > 180 || bounds.South < -90 || bounds.South > 90 || bounds.North < -90 || bounds.North > 90 || bounds.South >= bounds.North || bounds.West == bounds.East {
		return fmt.Errorf("coordinates are outside geographic bounds")
	}
	return nil
}

func validateRows(schema VisualizationDatasetSchema, columns []string, rows [][]any) error {
	if len(columns) == 0 {
		return fmt.Errorf("columns are required")
	}
	fields := make(map[string]VisualizationField, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.ID] = field
	}
	ordered := make([]VisualizationField, len(columns))
	seen := make(map[string]struct{}, len(columns))
	for index, column := range columns {
		field, ok := fields[column]
		if !ok {
			return fmt.Errorf("unknown column %q", column)
		}
		if _, exists := seen[column]; exists {
			return fmt.Errorf("duplicate column %q", column)
		}
		seen[column] = struct{}{}
		ordered[index] = field
	}
	for rowIndex, row := range rows {
		if len(row) != len(ordered) {
			return fmt.Errorf("row %d has width %d, want %d", rowIndex, len(row), len(ordered))
		}
		for columnIndex, value := range row {
			if err := validateScalar(ordered[columnIndex], value); err != nil {
				return fmt.Errorf("row %d column %q: %w", rowIndex, ordered[columnIndex].ID, err)
			}
		}
	}
	return nil
}

func validateScalar(field VisualizationField, value any) error {
	if value == nil {
		if field.Nullable {
			return nil
		}
		return fmt.Errorf("null is not allowed")
	}
	switch field.DataType {
	case VisualizationDataTypeString, VisualizationDataTypeTemporal, VisualizationDataTypeDate, VisualizationDataTypeGeographic:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string scalar, got %T", value)
		}
	case VisualizationDataTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean scalar, got %T", value)
		}
	case VisualizationDataTypeInteger:
		number, ok := scalarNumber(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
			return fmt.Errorf("expected finite integer scalar, got %v", value)
		}
	case VisualizationDataTypeDecimal:
		number, ok := scalarNumber(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("expected finite decimal scalar, got %v", value)
		}
	default:
		return fmt.Errorf("unsupported data type %q", field.DataType)
	}
	return nil
}

func scalarNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}
