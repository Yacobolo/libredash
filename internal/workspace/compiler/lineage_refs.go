package compiler

import (
	"regexp"
	"sort"

	semanticmodel "github.com/Yacobolo/leapview/internal/analytics/model"
	"github.com/Yacobolo/leapview/internal/workspace"
)

var lineageFieldRefPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\b`)

func fieldAssetID(modelID string, model *semanticmodel.Model, ref string, assetID func(workspace.AssetType, string) (workspace.AssetID, error)) (workspace.AssetID, error) {
	dimension, err := model.ResolveRelationshipEndpoint(ref)
	if err != nil {
		return "", err
	}
	return assetID(workspace.AssetTypeField, modelID+"."+dimension.Field)
}

func lineageMeasureFieldRefs(model *semanticmodel.Model, measure semanticmodel.MetricMeasure) []string {
	refs := []string{}
	if measure.Input.Field != "" {
		refs = append(refs, measure.Input.Field)
	}
	if measure.Input.Expression != "" {
		if expression, err := semanticmodel.ParseExpression(measure.Input.Expression); err == nil {
			refs = append(refs, expression.References()...)
		}
	}
	for _, filter := range measure.Filters {
		refs = append(refs, filter.Field)
	}
	sort.Strings(refs)
	return refs
}

func lineageExpressionFieldRefs(model *semanticmodel.Model, expression string) []string {
	seen := map[string]struct{}{}
	for _, match := range lineageFieldRefPattern.FindAllStringSubmatch(expression, -1) {
		ref := match[1] + "." + match[2]
		dimension, err := model.ResolveRelationshipEndpoint(ref)
		if err != nil {
			continue
		}
		seen[dimension.Field] = struct{}{}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}
