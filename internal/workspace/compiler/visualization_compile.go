package compiler

import (
	"fmt"
	"strings"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	dashboarddefinition "github.com/Yacobolo/leapview/internal/dashboard/definition"
	reportdef "github.com/Yacobolo/leapview/internal/dashboard/report"
	visualizationdefinition "github.com/Yacobolo/leapview/internal/visualization/definition"
	visualizationir "github.com/Yacobolo/leapview/internal/visualization/ir"
)

// compileContext is immutable per authored visualization. It keeps semantic
// model, renderer capability, and dataset identity at the compilation boundary
// instead of letting focused compilers rediscover them.
type compileContext struct {
	visualID   string
	modelID    string
	datasetID  string
	model      *semanticmodel.Model
	capability reportdef.VisualizationCapability
}

func newCompileContext(visualID, modelID, visualType string, model *semanticmodel.Model) (compileContext, error) {
	capability, ok := reportdef.VisualizationCapabilityForType(visualType)
	if !ok {
		return compileContext{}, fmt.Errorf("unsupported visualization type %q", visualType)
	}
	return compileContext{
		visualID: visualID, modelID: modelID, datasetID: "primary",
		model: model, capability: capability,
	}, nil
}

func CompileDashboardDefinition(authored *reportdef.Dashboard, visualizations map[string]visualizationdefinition.Definition) (dashboarddefinition.Definition, error) {
	filters := make(map[string]dashboarddefinition.FilterDefinition, len(authored.Filters))
	for id, filter := range authored.Filters {
		presets := make([]dashboarddefinition.FilterPreset, len(filter.Presets))
		for index, preset := range filter.Presets {
			presets[index] = dashboarddefinition.FilterPreset{Value: preset.Value, Label: preset.Label, From: preset.From, To: preset.To, RelativeDays: preset.RelativeDays}
		}
		options := make([]dashboarddefinition.FilterOption, len(filter.Options))
		for index, option := range filter.Options {
			options[index] = dashboarddefinition.FilterOption{Value: option.Value, Label: option.Label}
		}
		filters[id] = dashboarddefinition.FilterDefinition{
			Type: filter.Type, Label: filter.Label, Description: filter.Description, Dimension: filter.Dimension, Fact: filter.Fact,
			Default: dashboarddefinition.FilterDefault{Preset: filter.Default.Preset, From: filter.Default.From, To: filter.Default.To, Operator: filter.Default.Operator, Value: filter.Default.Value, Values: append([]string(nil), filter.Default.Values...)},
			Custom:  filter.Custom, Presets: presets, Operator: filter.Operator, Values: dashboarddefinition.FilterValues{Source: filter.Values.Source, Limit: filter.Values.Limit},
			DefaultOperator: filter.DefaultOperator, Operators: append([]string(nil), filter.Operators...), Options: options,
			URLParam: filter.URLParam, FromURLParam: filter.FromURLParam, ToURLParam: filter.ToURLParam, OperatorURLParam: filter.OperatorURLParam,
			Targets: dashboarddefinition.FilterTargets{Visuals: append([]string(nil), filter.Targets.Visuals...)},
		}
	}
	return dashboarddefinition.New(authored.ID, authored.Title, authored.Description, authored.SemanticModel, filters, authored.Pages, visualizations)
}

// compileVisualizationDefinitions is the one-way boundary from mutable YAML
// authoring objects to immutable renderer-independent serving definitions.
func compileVisualizationDefinitions(report *reportdef.Dashboard, models ...*semanticmodel.Model) (map[string]visualizationdefinition.Definition, error) {
	var model *semanticmodel.Model
	if len(models) > 0 {
		model = models[0]
	}
	out := make(map[string]visualizationdefinition.Definition, len(report.Visuals))
	for _, id := range sortedMapKeys(report.Visuals) {
		authoring := report.Visuals[id]
		ctx, err := newCompileContext(id, report.SemanticModel, authoring.Type, model)
		if err != nil {
			return nil, fmt.Errorf("visual %q: %w", id, err)
		}
		definition, err := compileAuthoringVisualization(ctx, authoring)
		if err != nil {
			return nil, fmt.Errorf("visual %q: %w", id, err)
		}
		out[id] = definition
	}
	return out, nil
}

func compileAuthoringVisualization(ctx compileContext, authoring reportdef.AuthoringVisualization) (visualizationdefinition.Definition, error) {
	if authoring.Tabular != nil {
		authored := *authoring.Tabular
		columns := compiledDashboardTableColumns(authoring.Type, authored, ctx.model)
		binding := compiledTableBinding(ctx.modelID, authoring.Type, authored)
		spec, err := compileTabularVisualizationSpec(ctx.visualID, authoring.Type, authored, columns, binding)
		if err != nil {
			return visualizationdefinition.Definition{}, err
		}
		return visualizationdefinition.New(ctx.visualID, spec, binding)
	}
	if authoring.Chart == nil {
		return visualizationdefinition.Definition{}, fmt.Errorf("visualization has no authoring variant")
	}
	authored := *authoring.Chart
	var (
		spec visualizationir.VisualizationSpec
		err  error
	)
	switch ctx.capability.Renderer {
	case visualizationdefinition.RendererVegaLite:
		spec, err = compileCustomVisualizationSpec(authored)
	case visualizationdefinition.RendererMapLibre:
		spec, err = compileGeographicVisualizationSpec(authored)
	default:
		spec, err = compileBuiltInVisualizationSpec(ctx.visualID, authored, ctx.model)
	}
	if err != nil {
		return visualizationdefinition.Definition{}, err
	}
	binding, err := compileVisualizationQueryBinding(ctx, authored)
	if err != nil {
		return visualizationdefinition.Definition{}, err
	}
	return visualizationdefinition.New(ctx.visualID, spec, binding)
}

func CompileVisualizationDefinitions(report *reportdef.Dashboard, models ...*semanticmodel.Model) (map[string]visualizationdefinition.Definition, error) {
	return compileVisualizationDefinitions(report, models...)
}

func compiledVisualTitle(authored reportdef.Visual, id string, model *semanticmodel.Model) string {
	if authored.Title != "" {
		return authored.Title
	}
	if model != nil && len(authored.Query.Measures) > 0 {
		measureID := authored.Query.Measures[0].Field
		if measure, err := model.ResolveMeasure(measureID); err == nil && strings.TrimSpace(measure.Label) != "" {
			return measure.Label
		}
		if metric, ok := model.Metrics[measureID]; ok && strings.TrimSpace(metric.Label) != "" {
			return metric.Label
		}
	}
	return id
}
