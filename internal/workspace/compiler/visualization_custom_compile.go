package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

func compileCustomVisualizationSpec(authored reportdef.Visual) (visualizationir.VisualizationSpec, error) {
	program, err := json.Marshal(authored.Custom.Program)
	if err != nil {
		return visualizationir.VisualizationSpec{}, fmt.Errorf("encode Vega-Lite program: %w", err)
	}
	fields := customVisualizationFields(authored.Query, authored.Interaction.PointSelection)
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field.ID] = struct{}{}
	}
	if err := validateCustomProgram(authored.Custom.Program, allowed, ""); err != nil {
		return visualizationir.VisualizationSpec{}, err
	}
	digest := sha256.Sum256(program)
	title := authored.Title
	if title == "" {
		title = "Custom visualization"
	}
	accessibilityTitle := authored.Accessibility.Title
	if accessibilityTitle == "" {
		accessibilityTitle = title
	}
	accessibilityDescription := authored.Accessibility.Description
	if accessibilityDescription == "" {
		accessibilityDescription = title
	}
	base := visualizationir.VisualizationSpecBase{
		Kind: "custom", Title: title, Datasets: []visualizationir.VisualizationDatasetSchema{{ID: "primary", Fields: fields}},
		DataBudget:    visualizationir.VisualizationDataBudget{MaxRows: compiledVisualLimit(authored), RequiredCompleteness: visualizationir.VisualizationCompletenessComplete},
		Accessibility: visualizationir.VisualizationAccessibility{Title: accessibilityTitle, Description: accessibilityDescription},
		Interactions:  customVisualizationInteractions(authored.Interaction.PointSelection),
	}
	return visualizationir.VisualizationSpec{Value: &visualizationir.CustomVisualizationSpec{
		VisualizationSpecBase: base, Kind: "custom", Engine: visualizationir.VisualizationCustomEngineVegaLite,
		Program: string(program), ProgramDigest: "sha256:" + hex.EncodeToString(digest[:]),
	}}, nil
}

func customVisualizationFields(query reportdef.VisualQuery, selection reportdef.SelectionInteraction) []visualizationir.VisualizationField {
	identity := map[string]bool{}
	for _, mapping := range selection.Mappings {
		identity[mapping.Value] = true
	}
	out := []visualizationir.VisualizationField{}
	appendField := func(field reportdef.FieldRef, role visualizationir.VisualizationFieldRole, dataType visualizationir.VisualizationDataType) {
		if field.Field == "" {
			return
		}
		alias := field.Alias
		if alias == "" {
			alias = fieldAlias(field.Field)
		}
		if identity[alias] {
			role = visualizationir.VisualizationFieldRoleIdentity
		}
		source := field.Field
		out = append(out, visualizationir.VisualizationField{ID: alias, SourceRef: &source, Role: role, DataType: dataType, Nullable: true, Label: alias})
	}
	for _, field := range query.Dimensions {
		appendField(field, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	}
	if query.Time.Field != "" {
		appendField(reportdef.FieldRef{Field: query.Time.Field, Alias: query.Time.Alias}, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	}
	appendField(query.Series, visualizationir.VisualizationFieldRoleDimension, visualizationir.VisualizationDataTypeString)
	for _, field := range query.Measures {
		appendField(field, visualizationir.VisualizationFieldRoleMeasure, visualizationir.VisualizationDataTypeDecimal)
	}
	return out
}

func customVisualizationInteractions(selection reportdef.SelectionInteraction) []visualizationir.VisualizationInteraction {
	return compiledSelectionInteractions("point_selection", selection)
}

func validateCustomProgram(value any, fields map[string]struct{}, path string) error {
	switch value := value.(type) {
	case []any:
		for index, item := range value {
			if err := validateCustomProgram(item, fields, fmt.Sprintf("%s/%d", path, index)); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range value {
			switch key {
			case "url", "href", "expr", "calculate", "transform", "params", "datasets", "values":
				return fmt.Errorf("Vega-Lite property %s/%s is not allowed", path, key)
			case "field":
				field, ok := item.(string)
				if !ok {
					return fmt.Errorf("Vega-Lite field at %s must be a string", path)
				}
				if _, ok := fields[field]; !ok {
					return fmt.Errorf("Vega-Lite field %q is not in the compiled dataset schema", field)
				}
			}
			if err := validateCustomProgram(item, fields, path+"/"+key); err != nil {
				return err
			}
		}
	}
	return nil
}
