package reportmodel

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/libredash/internal/analytics/model"
	"github.com/Yacobolo/libredash/internal/dashboard/report"
)

type SelectionScope string

const (
	SelectionScopeConformed SelectionScope = "conformed"
	SelectionScopeFactLocal SelectionScope = "fact_local"
)

type ResolvedSelectionInteraction struct {
	Mappings []ResolvedSelectionMapping
	Targets  []ResolvedSelectionTarget
}

type ResolvedSelectionMapping struct {
	Field string
	Fact  string
	Grain string
	Type  string
	Scope SelectionScope
}

type ResolvedSelectionTarget struct {
	Kind  string
	ID    string
	Facts []string
}

type SelectionMappingIdentity struct {
	Field string
	Fact  string
	Grain string
}

// CanonicalizeMappings validates that an incoming tuple contains each configured
// mapping identity exactly once and returns the mappings in authored order.
func (r ResolvedSelectionInteraction) CanonicalizeMappings(incoming []SelectionMappingIdentity) ([]ResolvedSelectionMapping, error) {
	if len(incoming) != len(r.Mappings) {
		return nil, fmt.Errorf("selection tuple has %d mappings; want %d", len(incoming), len(r.Mappings))
	}
	configured := make(map[SelectionMappingIdentity]ResolvedSelectionMapping, len(r.Mappings))
	for _, mapping := range r.Mappings {
		identity := SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		configured[identity] = mapping
	}
	seen := make(map[SelectionMappingIdentity]bool, len(incoming))
	for _, identity := range incoming {
		if _, ok := configured[identity]; !ok {
			return nil, fmt.Errorf("selection tuple contains unknown mapping identity field=%q fact=%q grain=%q", identity.Field, identity.Fact, identity.Grain)
		}
		if seen[identity] {
			return nil, fmt.Errorf("selection tuple contains duplicate mapping identity field=%q fact=%q grain=%q", identity.Field, identity.Fact, identity.Grain)
		}
		seen[identity] = true
	}
	canonical := make([]ResolvedSelectionMapping, len(r.Mappings))
	copy(canonical, r.Mappings)
	return canonical, nil
}

// ResolveSelectionInteraction resolves and validates the configured interaction
// for a report source. Its mapping order is canonical and matches the authored
// order, so callers can use the result to validate command tuples exactly.
func ResolveSelectionInteraction(d *report.Dashboard, model *semanticmodel.Model, sourceKind, sourceID string) (ResolvedSelectionInteraction, error) {
	selection, err := sourceSelection(d, sourceKind, sourceID)
	if err != nil {
		return ResolvedSelectionInteraction{}, err
	}
	exposed, time := sourceSelectionFields(d, sourceKind, sourceID)
	resolved := ResolvedSelectionInteraction{Mappings: make([]ResolvedSelectionMapping, 0, len(selection.Mappings))}
	for index, mapping := range selection.Mappings {
		item, err := resolveSelectionMapping(model, mapping)
		if err != nil {
			return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction mapping %d: %w", sourceKind, sourceID, index, err)
		}
		if !exposed[mapping.Field] {
			return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction mapping %d field %q is not exposed by the source query", sourceKind, sourceID, index, mapping.Field)
		}
		if err := validateSelectionGrain(mapping, item.Type, time); err != nil {
			return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction mapping %d: %w", sourceKind, sourceID, index, err)
		}
		resolved.Mappings = append(resolved.Mappings, item)
	}
	if err := validateSelectionTupleScope(resolved.Mappings); err != nil {
		return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction mappings %w", sourceKind, sourceID, err)
	}
	if err := validateSelectionSourceFacts(d, model, sourceKind, sourceID, resolved.Mappings); err != nil {
		return ResolvedSelectionInteraction{}, err
	}
	for _, targetID := range selection.Targets {
		targetKind, err := selectionTargetKind(d, targetID)
		if err != nil {
			return ResolvedSelectionInteraction{}, err
		}
		facts, err := TargetFacts(d, model, targetKind, targetID)
		if err != nil {
			return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction target %q: %w", sourceKind, sourceID, targetID, err)
		}
		if err := validateSelectionTarget(model, targetID, facts, resolved.Mappings); err != nil {
			return ResolvedSelectionInteraction{}, fmt.Errorf("%s %q interaction: %w", sourceKind, sourceID, err)
		}
		resolved.Targets = append(resolved.Targets, ResolvedSelectionTarget{Kind: targetKind, ID: targetID, Facts: facts})
	}
	return resolved, nil
}

func sourceSelection(d *report.Dashboard, sourceKind, sourceID string) (report.SelectionInteraction, error) {
	switch sourceKind {
	case "visual":
		if visual, ok := d.Visuals[sourceID]; ok {
			return visual.Interaction.PointSelection, nil
		}
		if visual, ok := d.Tables[sourceID]; ok {
			return visual.Interaction.RowSelection, nil
		}
		return report.SelectionInteraction{}, fmt.Errorf("unknown source visual %q", sourceID)
	default:
		return report.SelectionInteraction{}, fmt.Errorf("unknown source kind %q", sourceKind)
	}
}

func sourceSelectionFields(d *report.Dashboard, sourceKind, sourceID string) (map[string]bool, report.QueryTime) {
	fields := map[string]bool{}
	if sourceKind == "visual" {
		if visual, ok := d.Visuals[sourceID]; ok {
			for _, dimension := range visual.Query.Dimensions {
				fields[dimension.Field] = true
			}
			if !visual.Query.Series.IsZero() {
				fields[visual.Query.Series.Field] = true
			}
			if visual.Query.Time.Field != "" {
				fields[visual.Query.Time.Field] = true
			}
			return fields, visual.Query.Time
		}
		if table, ok := d.Tables[sourceID]; ok {
			for _, field := range table.Query.Fields {
				fields[field] = true
			}
			for _, columns := range [][]report.FieldRef{table.Query.Columns, table.Query.Rows} {
				for _, field := range columns {
					fields[field.Field] = true
				}
			}
		}
		return fields, report.QueryTime{}
	}
	table := d.Tables[sourceID]
	for _, field := range table.Query.Fields {
		fields[field] = true
	}
	for _, columns := range [][]report.FieldRef{table.Query.Columns, table.Query.Rows} {
		for _, field := range columns {
			fields[field.Field] = true
		}
	}
	return fields, report.QueryTime{}
}

func resolveSelectionMapping(model *semanticmodel.Model, mapping report.SelectionMapping) (ResolvedSelectionMapping, error) {
	if !strings.Contains(mapping.Field, ".") {
		dimension, err := model.ResolveSemanticDimension(mapping.Field)
		if err != nil {
			return ResolvedSelectionMapping{}, err
		}
		if mapping.Fact != "" {
			return ResolvedSelectionMapping{}, fmt.Errorf("semantic dimension %q must not specify fact", mapping.Field)
		}
		if mapping.Grain != "" && !containsString(dimension.Grains, mapping.Grain) {
			return ResolvedSelectionMapping{}, fmt.Errorf("semantic dimension %q does not support grain %q", mapping.Field, mapping.Grain)
		}
		return ResolvedSelectionMapping{Field: mapping.Field, Grain: mapping.Grain, Type: dimension.Type, Scope: SelectionScopeConformed}, nil
	}
	if mapping.Fact == "" {
		return ResolvedSelectionMapping{}, fmt.Errorf("physical field %q requires fact", mapping.Field)
	}
	if _, ok := model.Tables[mapping.Fact]; !ok {
		return ResolvedSelectionMapping{}, fmt.Errorf("physical field %q references unknown fact %q", mapping.Field, mapping.Fact)
	}
	dimension, err := model.ResolveDimension(mapping.Field)
	if err != nil {
		return ResolvedSelectionMapping{}, err
	}
	if err := model.CanReachField(mapping.Fact, mapping.Field); err != nil {
		return ResolvedSelectionMapping{}, err
	}
	return ResolvedSelectionMapping{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain, Type: dimension.Type, Scope: SelectionScopeFactLocal}, nil
}

func validateSelectionGrain(mapping report.SelectionMapping, fieldType string, time report.QueryTime) error {
	if time.Field == mapping.Field {
		if fieldType != "date" && fieldType != "timestamp" {
			return fmt.Errorf("field %q type %q cannot be used as a grained time selection", mapping.Field, fieldType)
		}
		if mapping.Grain != time.Grain {
			return fmt.Errorf("field %q requires grain %q to match the source query", mapping.Field, time.Grain)
		}
		return nil
	}
	if mapping.Grain != "" {
		return fmt.Errorf("field %q grain is only valid for a grained query time field", mapping.Field)
	}
	return nil
}

func validateSelectionTupleScope(mappings []ResolvedSelectionMapping) error {
	seen := map[SelectionMappingIdentity]bool{}
	for _, mapping := range mappings {
		identity := SelectionMappingIdentity{Field: mapping.Field, Fact: mapping.Fact, Grain: mapping.Grain}
		if seen[identity] {
			return fmt.Errorf("contains duplicate mapping identity field=%q fact=%q grain=%q", mapping.Field, mapping.Fact, mapping.Grain)
		}
		seen[identity] = true
	}
	if len(mappings) < 2 {
		return nil
	}
	scope, fact := mappings[0].Scope, mappings[0].Fact
	for _, mapping := range mappings[1:] {
		if mapping.Scope != scope || (scope == SelectionScopeFactLocal && mapping.Fact != fact) {
			return fmt.Errorf("must be entirely conformed or fact-local to one fact")
		}
	}
	return nil
}

func validateSelectionSourceFacts(d *report.Dashboard, model *semanticmodel.Model, sourceKind, sourceID string, mappings []ResolvedSelectionMapping) error {
	facts, err := TargetFacts(d, model, sourceKind, sourceID)
	if err != nil {
		return fmt.Errorf("%s %q interaction source facts: %w", sourceKind, sourceID, err)
	}
	return validateSelectionCompatibility(model, "source", sourceID, facts, mappings)
}

func validateSelectionTarget(model *semanticmodel.Model, targetID string, facts []string, mappings []ResolvedSelectionMapping) error {
	return validateSelectionCompatibility(model, "target", targetID, facts, mappings)
}

func validateSelectionCompatibility(model *semanticmodel.Model, role, id string, facts []string, mappings []ResolvedSelectionMapping) error {
	for _, mapping := range mappings {
		switch mapping.Scope {
		case SelectionScopeConformed:
			dimension := model.Dimensions[mapping.Field]
			for _, fact := range facts {
				if _, ok := dimension.Bindings[fact]; !ok {
					return fmt.Errorf("semantic dimension %q has no binding for %s fact %q", mapping.Field, role, fact)
				}
			}
		case SelectionScopeFactLocal:
			if !containsFact(facts, mapping.Fact) {
				return fmt.Errorf("%s %q does not participate in fact %q", role, id, mapping.Fact)
			}
		}
	}
	return nil
}

func selectionTargetKind(d *report.Dashboard, targetID string) (string, error) {
	_, visualOK := d.Visuals[targetID]
	_, tableOK := d.Tables[targetID]
	if visualOK == tableOK {
		if visualOK {
			return "", fmt.Errorf("interaction target %q is ambiguous across visuals and tables", targetID)
		}
		return "", fmt.Errorf("interaction references unknown target %q", targetID)
	}
	if visualOK {
		return "visual", nil
	}
	return "table", nil
}

func containsFact(facts []string, fact string) bool {
	for _, candidate := range facts {
		if candidate == fact {
			return true
		}
	}
	return false
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
